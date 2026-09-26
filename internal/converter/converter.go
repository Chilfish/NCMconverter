// Package converter decrypts the sections of an NCM container into playable
// audio and the metadata that describes it.
package converter

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/chilfish/ncmconverter/internal/ncm"
)

// Prefixes and masks used by the container format.
const (
	// keyPrefix begins the decrypted key section.
	keyPrefix = "neteasecloudmusic"
	// metaPrefix begins the de-obfuscated meta section, before base64 decoding.
	metaPrefix = "163 key(Don't modify):"
	// metaJSONPrefix begins the meta payload after decryption.
	metaJSONPrefix = "music:"
	// keyXorMask and metaXorMask are applied to the key and meta sections
	// before they are decrypted.
	keyXorMask  = 0x64
	metaXorMask = 0x63
	// musicChunkSize is the block the audio frames are obfuscated in. The
	// keystream position restarts at the beginning of every block.
	musicChunkSize = 0x8000
)

// Audio container formats this package reports.
const (
	FormatMP3  = "mp3"
	FormatFLAC = "flac"
)

// ErrUnknownFormat reports audio frames whose container could be identified
// neither from the metadata nor from the frame contents.
var ErrUnknownFormat = errors.New("could not determine audio format")

var (
	// aesCoreKey decrypts the key section.
	aesCoreKey = []byte{0x68, 0x7A, 0x48, 0x52, 0x41, 0x6D, 0x73, 0x6F, 0x35, 0x6B, 0x49, 0x6E, 0x62, 0x61, 0x78, 0x57}
	// aesModifyKey decrypts the meta section.
	aesModifyKey = []byte{0x23, 0x31, 0x34, 0x6C, 0x6A, 0x6B, 0x5F, 0x21, 0x5C, 0x5D, 0x26, 0x30, 0x55, 0x3C, 0x27, 0x28}

	flacMagic = []byte("fLaC")
	id3Magic  = []byte("ID3")
)

// Converter decodes the sections of an NCM container.
type Converter struct {
	*ncm.File

	// KeyData is the decrypted key material, set by HandleKey.
	KeyData []byte
	// MetaData is the decoded track metadata, set by HandleMeta.
	MetaData *Meta
	// MusicData is the de-obfuscated audio, set by HandleMusic.
	MusicData []byte
}

// NewConverter wraps an already-opened container.
func NewConverter(file *ncm.File) *Converter {
	return &Converter{File: file}
}

// HandleAll runs every conversion step, in the order they depend on each other.
func (c *Converter) HandleAll() error {
	if err := c.HandleKey(); err != nil {
		return err
	}
	if err := c.HandleMeta(); err != nil {
		return err
	}
	if err := c.HandleMusic(); err != nil {
		return err
	}
	return c.resolveFormat()
}

// HandleKey decrypts the key section into KeyData.
func (c *Converter) HandleKey() error {
	obfuscated := make([]byte, len(c.Key.Bytes))
	for i, b := range c.Key.Bytes {
		obfuscated[i] = b ^ keyXorMask
	}

	key, err := decryptAES128ECB(aesCoreKey, obfuscated)
	if err != nil {
		return fmt.Errorf("decrypt key section: %w", err)
	}
	if !bytes.HasPrefix(key, []byte(keyPrefix)) {
		return fmt.Errorf("key section does not begin with %q", keyPrefix)
	}
	c.KeyData = key
	return nil
}

// HandleMeta decrypts the meta section into MetaData.
//
// Containers without a meta section are legal. The format of such a file is
// determined from its audio frames instead.
func (c *Converter) HandleMeta() error {
	if c.Meta.Length == 0 {
		c.MetaData = &Meta{}
		return nil
	}

	obfuscated := make([]byte, len(c.Meta.Bytes))
	for i, b := range c.Meta.Bytes {
		obfuscated[i] = b ^ metaXorMask
	}
	if !bytes.HasPrefix(obfuscated, []byte(metaPrefix)) {
		return fmt.Errorf("meta section does not begin with %q", metaPrefix)
	}

	encoded := obfuscated[len(metaPrefix):]
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	n, err := base64.StdEncoding.Decode(decoded, encoded)
	if err != nil {
		return fmt.Errorf("decode meta section: %w", err)
	}

	plain, err := decryptAES128ECB(aesModifyKey, decoded[:n])
	if err != nil {
		return fmt.Errorf("decrypt meta section: %w", err)
	}
	if !bytes.HasPrefix(plain, []byte(metaJSONPrefix)) {
		return fmt.Errorf("meta payload does not begin with %q", metaJSONPrefix)
	}

	// The payload carries the track fields and the album fields together.
	payload := plain[len(metaJSONPrefix):]
	var meta Meta
	if err := json.Unmarshal(payload, &meta); err != nil {
		return fmt.Errorf("decode track metadata: %w", err)
	}
	var album Album
	if err := json.Unmarshal(payload, &album); err != nil {
		return fmt.Errorf("decode album metadata: %w", err)
	}
	meta.Album = &album
	meta.Comment = string(obfuscated)
	c.MetaData = &meta
	return nil
}

// HandleMusic de-obfuscates the audio frames using the key material that
// HandleKey produced.
func (c *Converter) HandleMusic() error {
	if len(c.KeyData) <= len(keyPrefix) {
		return errors.New("key has not been resolved yet")
	}
	box, err := buildKeyBox(c.KeyData[len(keyPrefix):])
	if err != nil {
		return fmt.Errorf("derive key box: %w", err)
	}

	source := c.Music.Bytes
	decoded := make([]byte, len(source))
	for off := 0; off < len(source); off += musicChunkSize {
		end := min(off+musicChunkSize, len(source))
		for i := off; i < end; i++ {
			// Byte arithmetic already wraps at 256, which is what keeps these
			// lookups inside the 256 byte box.
			j := byte(i - off + 1)
			k := box[j]
			decoded[i] = source[i] ^ box[k+box[k+j]]
		}
	}
	c.MusicData = decoded
	return nil
}

// resolveFormat records the audio container format in the metadata, preferring
// the format the container declared over one sniffed from the audio frames.
func (c *Converter) resolveFormat() error {
	if c.MetaData == nil {
		c.MetaData = &Meta{}
	}
	if format := strings.ToLower(c.MetaData.Format); format != "" {
		c.MetaData.Format = format
		return nil
	}

	format, err := formatFromMagic(c.MusicData)
	if err != nil {
		return err
	}
	c.MetaData.Format = format
	return nil
}

// formatFromMagic identifies an audio container from its leading bytes.
func formatFromMagic(data []byte) (string, error) {
	switch {
	case bytes.HasPrefix(data, flacMagic):
		return FormatFLAC, nil
	case bytes.HasPrefix(data, id3Magic):
		return FormatMP3, nil
	case len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0:
		// An MPEG audio frame starts with an eleven bit sync word.
		return FormatMP3, nil
	default:
		return "", fmt.Errorf("%w: leading bytes % X", ErrUnknownFormat, data[:min(4, len(data))])
	}
}
