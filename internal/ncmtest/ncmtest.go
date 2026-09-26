// Package ncmtest builds synthetic NCM containers for tests.
//
// The builders deliberately reimplement the container format instead of reusing
// the packages under test, so that a bug in a reader cannot also hide itself in
// the fixture that is meant to catch it.
package ncmtest

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
)

// Key material and prefixes defined by the NCM format.
var (
	coreKey   = []byte{0x68, 0x7A, 0x48, 0x52, 0x41, 0x6D, 0x73, 0x6F, 0x35, 0x6B, 0x49, 0x6E, 0x62, 0x61, 0x78, 0x57}
	modifyKey = []byte{0x23, 0x31, 0x34, 0x6C, 0x6A, 0x6B, 0x5F, 0x21, 0x5C, 0x5D, 0x26, 0x30, 0x55, 0x3C, 0x27, 0x28}
)

const (
	// KeyPrefix begins the key section.
	KeyPrefix = "neteasecloudmusic"
	// MetaPrefix begins the meta section before base64 decoding.
	MetaPrefix = "163 key(Don't modify):"
	// MetaJSONPrefix begins the meta payload after decryption.
	MetaJSONPrefix = "music:"

	keyXorMask     = 0x64
	metaXorMask    = 0x63
	musicBlockSize = 0x8000

	// MagicHeader is the ASCII magic at the start of every container.
	MagicHeader = "CTENFDAM"
)

// Options describes the container that Build assembles.
type Options struct {
	// KeyMaterial is appended to KeyPrefix and encrypted into the key section.
	// It defaults to a fixed value when empty.
	KeyMaterial []byte
	// Meta is marshalled to JSON and stored in the meta section. A nil value
	// builds a container whose meta section is empty.
	Meta any
	// RawMeta is stored in the meta section verbatim, bypassing Meta. It exists
	// so that tests can build containers with a deliberately malformed meta
	// section.
	RawMeta []byte
	// Cover is stored verbatim in the cover section.
	Cover []byte
	// Music is the plaintext audio, obfuscated into the music section.
	Music []byte
}

// Build assembles a complete NCM container.
func Build(opts Options) []byte {
	if len(opts.KeyMaterial) == 0 {
		opts.KeyMaterial = []byte("0123456789abcdef")
	}
	key := append([]byte(KeyPrefix), opts.KeyMaterial...)

	keySection := xorBytes(mustEncrypt(coreKey, key), keyXorMask)

	var metaSection []byte
	switch {
	case opts.RawMeta != nil:
		metaSection = opts.RawMeta
	case opts.Meta != nil:
		payload, err := json.Marshal(opts.Meta)
		if err != nil {
			panic(fmt.Sprintf("ncmtest: marshal meta: %v", err))
		}
		encrypted := mustEncrypt(modifyKey, append([]byte(MetaJSONPrefix), payload...))
		encoded := base64.StdEncoding.EncodeToString(encrypted)
		metaSection = xorBytes([]byte(MetaPrefix+encoded), metaXorMask)
	}

	var out []byte
	out = append(out, MagicHeader...)
	out = append(out, 0x00, 0x00) // padding, ignored by readers
	out = binary.LittleEndian.AppendUint32(out, uint32(len(keySection)))
	out = append(out, keySection...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(metaSection)))
	out = append(out, metaSection...)
	out = binary.LittleEndian.AppendUint32(out, 0x11223344)              // CRC32 of the meta section
	out = append(out, 0x01)                                              // meta section version
	out = binary.LittleEndian.AppendUint32(out, uint32(len(opts.Cover))) // reserved
	out = binary.LittleEndian.AppendUint32(out, uint32(len(opts.Cover)))
	out = append(out, opts.Cover...)
	out = append(out, obfuscate(opts.Music, buildKeyBox(key[len(KeyPrefix):]))...)
	return out
}

// PNG returns a tiny but genuinely decodable PNG image. Cover art handling
// decodes the image to record its dimensions, so arbitrary bytes will not do as
// a fixture.
func PNG() []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		panic(fmt.Sprintf("ncmtest: encode png: %v", err))
	}
	return buf.Bytes()
}

// JPEG returns a tiny but genuinely decodable JPEG image.
func JPEG() []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		panic(fmt.Sprintf("ncmtest: encode jpeg: %v", err))
	}
	return buf.Bytes()
}

// MinimalMP3 returns audio that begins with an empty ID3v2.3 tag followed by
// the given frame bytes.
//
// The tag version matters: audio held by a container already carries an ID3v2.3
// tag, and a tagger has to cope with the text encoding that version implies.
func MinimalMP3(frames []byte) []byte {
	tag := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 0}
	return append(tag, frames...)
}

// MinimalFLAC returns the shortest stream go-flac accepts: a stream info
// metadata block followed by one frame beginning with the frame sync code.
func MinimalFLAC(frames []byte) []byte {
	var info [34]byte
	binary.BigEndian.PutUint16(info[0:], 4096) // minimum block size
	binary.BigEndian.PutUint16(info[2:], 4096) // maximum block size
	binary.BigEndian.PutUint16(info[4:], 0)    // minimum frame size
	binary.BigEndian.PutUint16(info[6:], 0)    // maximum frame size
	var packed uint64
	packed |= uint64(44100) << 44 // sample rate
	packed |= uint64(1) << 41     // channels - 1, so two channels
	packed |= uint64(15) << 36    // bits per sample - 1, so sixteen bits
	binary.BigEndian.PutUint64(info[10:], packed)

	out := []byte("fLaC")
	out = append(out, 0x80, 0x00, 0x00, 0x22) // last metadata block, StreamInfo, 34 bytes
	out = append(out, info[:]...)
	out = append(out, 0xff, 0xf8) // FLAC frame sync code
	return append(out, frames...)
}

// mustEncrypt pads plain with PKCS#7 and encrypts it with AES-128 in ECB mode.
func mustEncrypt(key, plain []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(fmt.Sprintf("ncmtest: create cipher: %v", err))
	}
	blockSize := block.BlockSize()
	padding := blockSize - len(plain)%blockSize
	padded := make([]byte, len(plain)+padding)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += blockSize {
		block.Encrypt(out[i:i+blockSize], padded[i:i+blockSize])
	}
	return out
}

// xorBytes applies a single byte mask to data.
func xorBytes(data []byte, mask byte) []byte {
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ mask
	}
	return out
}

// buildKeyBox derives the audio obfuscation box from the key material.
func buildKeyBox(key []byte) []byte {
	box := make([]byte, 256)
	for i := range box {
		box[i] = byte(i)
	}
	var lastByte byte
	keyOffset := 0
	for i := range box {
		c := box[i] + lastByte + key[keyOffset]
		keyOffset++
		if keyOffset >= len(key) {
			keyOffset = 0
		}
		box[i], box[c] = box[c], box[i]
		lastByte = c
	}
	return box
}

// obfuscate applies the audio cipher, which is its own inverse.
func obfuscate(data, box []byte) []byte {
	out := make([]byte, len(data))
	for off := 0; off < len(data); off += musicBlockSize {
		end := min(off+musicBlockSize, len(data))
		for i := off; i < end; i++ {
			j := byte(i - off + 1)
			k := box[j]
			out[i] = data[i] ^ box[k+box[k+j]]
		}
	}
	return out
}
