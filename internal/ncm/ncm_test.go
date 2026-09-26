package ncm_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chilfish/ncmconverter/internal/ncm"
	"github.com/chilfish/ncmconverter/internal/ncmtest"
)

// openContainer writes a container to a temporary file and opens it.
func openContainer(t *testing.T, data []byte, ext string) *ncm.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample"+ext)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}
	file, err := ncm.Open(path)
	if err != nil {
		t.Fatalf("open container: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestParseReadsEverySection(t *testing.T) {
	cover := bytes.Repeat([]byte{0xff, 0xd8, 0xff, 0xe0}, 7)
	music := bytes.Repeat([]byte{0x5a}, 4096)

	file := openContainer(t, ncmtest.Build(ncmtest.Options{
		Meta:  map[string]any{"musicId": 1, "musicName": "Track", "format": "mp3"},
		Cover: cover,
		Music: music,
	}), ".ncm")

	if err := file.Parse(); err != nil {
		t.Fatalf("parse container: %v", err)
	}

	if len(file.Key.Bytes) == 0 || file.Key.Length != uint64(len(file.Key.Bytes)) {
		t.Errorf("key section = %d bytes, length %d", len(file.Key.Bytes), file.Key.Length)
	}
	if len(file.Meta.Bytes) == 0 || file.Meta.Length != uint64(len(file.Meta.Bytes)) {
		t.Errorf("meta section = %d bytes, length %d", len(file.Meta.Bytes), file.Meta.Length)
	}
	// The cover and music lengths are what prove the offsets used to locate
	// them are right: a misaligned offset reports a nonsense length.
	if file.Cover.Length != uint64(len(cover)) {
		t.Errorf("cover length = %d, want %d", file.Cover.Length, len(cover))
	}
	if !bytes.Equal(file.Cover.Bytes, cover) {
		t.Error("cover bytes do not match the container contents")
	}
	if file.Music.Length != uint64(len(music)) {
		t.Errorf("music length = %d, want %d", file.Music.Length, len(music))
	}
}

// TestValidateRejectsWrongMagicNumbers checks each magic number on its own.
//
// Corrupting the first number while leaving the second intact used to be
// accepted, because the check combined them with "and" instead of "or".
func TestValidateRejectsWrongMagicNumbers(t *testing.T) {
	valid := ncmtest.Build(ncmtest.Options{
		Meta:  map[string]any{"format": "mp3"},
		Music: bytes.Repeat([]byte{0x11}, 128),
	})

	tests := []struct {
		name  string
		index int
	}{
		{"first magic number", 0},
		{"second magic number", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrupted := bytes.Clone(valid)
			corrupted[tt.index] ^= 0xff

			file := openContainer(t, corrupted, ".ncm")
			if err := file.Parse(); !errors.Is(err, ncm.ErrMagicHeader) {
				t.Fatalf("parse error = %v, want %v", err, ncm.ErrMagicHeader)
			}
		})
	}
}

func TestValidateRejectsNonNCMExtension(t *testing.T) {
	data := ncmtest.Build(ncmtest.Options{
		Meta:  map[string]any{"format": "mp3"},
		Music: bytes.Repeat([]byte{0x11}, 128),
	})

	file := openContainer(t, data, ".bin")
	if err := file.Parse(); !errors.Is(err, ncm.ErrExtNcm) {
		t.Fatalf("parse error = %v, want %v", err, ncm.ErrExtNcm)
	}
}

// TestParseRejectsTruncatedContainer makes sure a section whose declared length
// runs past the end of the file fails instead of silently reading what is left.
func TestParseRejectsTruncatedContainer(t *testing.T) {
	cover := bytes.Repeat([]byte{0xab}, 512)
	music := bytes.Repeat([]byte{0xcd}, 1024)
	data := ncmtest.Build(ncmtest.Options{
		Meta:  map[string]any{"format": "mp3"},
		Cover: cover,
		Music: music,
	})

	truncated := data[:len(data)-len(music)-len(cover)/2]

	file := openContainer(t, truncated, ".ncm")
	if err := file.Parse(); err == nil {
		t.Fatal("parse succeeded on a container truncated inside the cover, want an error")
	}
}

func TestOpenMissingFile(t *testing.T) {
	if _, err := ncm.Open(filepath.Join(t.TempDir(), "absent.ncm")); err == nil {
		t.Fatal("open succeeded on a missing file, want an error")
	}
}
