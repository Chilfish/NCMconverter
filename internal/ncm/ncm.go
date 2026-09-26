// Package ncm reads NetEase Cloud Music (.ncm) container files and splits them
// into the key, metadata, cover art and encrypted audio sections they hold.
//
// A container is a sequence of length-prefixed sections. Every section except
// the leading magic header is prefixed with a little-endian uint32 holding the
// size of the bytes that follow it:
//
//	offset           size       content
//	0                8          magic header, the ASCII bytes "CTENFDAM"
//	8                2          padding
//	10               4          key section length
//	14               keyLen     key section
//	14+keyLen        4          meta section length
//	18+keyLen        metaLen    meta section
//	metaEnd          4          checksum over the meta section, not validated
//	metaEnd+4        1          meta section version, always 0x01
//	metaEnd+5        4          reserved, observed to echo the cover length
//	metaEnd+9        4          cover section length
//	metaEnd+13       coverLen   cover section, a JPEG or PNG image
//	metaEnd+13+coverLen  rest   encrypted audio frames
//
// The layout above was verified against real containers; the "reserved" field
// at metaEnd+5 is not read, but the cover length that follows it is.
//
// The four bytes at metaEnd are not a CRC32 of the meta section, whatever the
// format's documentation says. On a real container they read 0xf372d5d0, while
// hash/crc32.ChecksumIEEE over the stored meta bytes gives 0x05f35836 — and
// neither the de-obfuscated text nor the decrypted payload matches either. The
// field's definition is unknown, so it is deliberately left unchecked rather
// than compared against a value we would be guessing at.
package ncm

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MagicHeader1 and MagicHeader2 identify an NCM container. They are the ASCII
// bytes "CTEN" and "FDAM" read as little-endian uint32 values.
const (
	MagicHeader1 uint32 = 0x4e455443
	MagicHeader2 uint32 = 0x4d414446
)

// Fixed sizes of the fields that make up a container header.
const (
	magicHeaderSize   = 8
	headerPaddingSize = 2
	lengthFieldSize   = 4
	// metaTrailingSize covers the meta checksum (4 bytes) and the version byte
	// (1 byte) that separate the meta section from the cover section.
	metaTrailingSize = 5
	// coverReservedSize covers a field that is not read. The cover length sits
	// immediately after it, which is why it has to be accounted for.
	coverReservedSize = 4
)

// Section is one length-prefixed chunk of a container.
type Section struct {
	Length uint64
	Bytes  []byte
}

// File is an NCM container opened from disk.
type File struct {
	Path     string
	FileDir  string
	FileName string
	Ext      string

	Key   Section
	Meta  Section
	Cover Section
	Music Section

	fd *os.File
}

// Open opens the NCM container at path.
//
// The returned File holds an open handle and must be closed by the caller.
func Open(path string) (*File, error) {
	cleaned := filepath.Clean(path)
	fd, err := os.Open(cleaned)
	if err != nil {
		return nil, fmt.Errorf("open ncm file: %w", err)
	}
	return &File{
		Path:     cleaned,
		FileDir:  filepath.Dir(cleaned),
		FileName: filepath.Base(cleaned),
		Ext:      filepath.Ext(cleaned),
		fd:       fd,
	}, nil
}

// Close releases the underlying file handle.
func (f *File) Close() error {
	return f.fd.Close()
}

// Parse reads every section of the container.
//
// The sections have to be read in order, because the offset of each one depends
// on the size of the sections before it.
func (f *File) Parse() error {
	if err := f.Validate(); err != nil {
		return err
	}
	if err := f.GetKey(); err != nil {
		return fmt.Errorf("read key section: %w", err)
	}
	if err := f.GetMeta(); err != nil {
		return fmt.Errorf("read meta section: %w", err)
	}
	if err := f.GetCover(); err != nil {
		return fmt.Errorf("read cover section: %w", err)
	}
	if err := f.GetMusicData(); err != nil {
		return fmt.Errorf("read music section: %w", err)
	}
	return nil
}

// Validate reports whether the file carries the .ncm extension and the magic
// header of an NCM container.
func (f *File) Validate() error {
	if !strings.EqualFold(f.Ext, ".ncm") {
		return ErrExtNcm
	}
	return f.checkHeader()
}

// checkHeader verifies both magic numbers at the start of the file.
func (f *File) checkHeader() error {
	if _, err := f.fd.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek to header: %w", err)
	}
	m1, err := readUint[uint32](f.fd)
	if err != nil {
		return fmt.Errorf("read first magic number: %w", err)
	}
	m2, err := readUint[uint32](f.fd)
	if err != nil {
		return fmt.Errorf("read second magic number: %w", err)
	}
	if m1 != MagicHeader1 || m2 != MagicHeader2 {
		return ErrMagicHeader
	}
	return nil
}

// keyOffset is where the length prefix of the key section begins.
func (f *File) keyOffset() int64 {
	return magicHeaderSize + headerPaddingSize
}

// metaOffset is where the length prefix of the meta section begins.
func (f *File) metaOffset() int64 {
	return f.keyOffset() + lengthFieldSize + int64(f.Key.Length)
}

// coverOffset is where the length prefix of the cover section begins.
func (f *File) coverOffset() int64 {
	return f.metaOffset() + lengthFieldSize + int64(f.Meta.Length) +
		metaTrailingSize + coverReservedSize
}

// musicOffset is where the encrypted audio frames begin.
func (f *File) musicOffset() int64 {
	return f.coverOffset() + lengthFieldSize + int64(f.Cover.Length)
}

// section reads the length-prefixed section whose length prefix sits at off.
func (f *File) section(off int64) (Section, error) {
	if _, err := f.fd.Seek(off, io.SeekStart); err != nil {
		return Section{}, fmt.Errorf("seek to section at %d: %w", off, err)
	}
	length, err := readUint[uint32](f.fd)
	if err != nil {
		return Section{}, fmt.Errorf("read section length at %d: %w", off, err)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(f.fd, buf); err != nil {
		return Section{}, fmt.Errorf("read %d byte section at %d: %w", length, off, err)
	}
	return Section{Length: uint64(length), Bytes: buf}, nil
}

// GetKey reads the key section. It must be called before GetMeta.
func (f *File) GetKey() error {
	section, err := f.section(f.keyOffset())
	if err != nil {
		return err
	}
	f.Key = section
	return nil
}

// GetMeta reads the meta section. It must be called after GetKey.
func (f *File) GetMeta() error {
	section, err := f.section(f.metaOffset())
	if err != nil {
		return err
	}
	f.Meta = section
	return nil
}

// GetCover reads the cover section. It must be called after GetMeta.
func (f *File) GetCover() error {
	section, err := f.section(f.coverOffset())
	if err != nil {
		return err
	}
	f.Cover = section
	return nil
}

// GetMusicData reads the encrypted audio frames that follow the cover section.
// It must be called after GetCover.
func (f *File) GetMusicData() error {
	off := f.musicOffset()
	if _, err := f.fd.Seek(off, io.SeekStart); err != nil {
		return fmt.Errorf("seek to music section at %d: %w", off, err)
	}
	data, err := io.ReadAll(f.fd)
	if err != nil {
		return fmt.Errorf("read music section at %d: %w", off, err)
	}
	f.Music = Section{Length: uint64(len(data)), Bytes: data}
	return nil
}
