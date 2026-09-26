package converter

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/chilfish/ncmconverter/internal/ncm"
	"github.com/chilfish/ncmconverter/internal/ncmtest"
)

// sampleMeta is the metadata used by the container-level tests.
func sampleMeta() map[string]any {
	return map[string]any{
		"musicId":   2611651882,
		"musicName": "Re:Re:",
		"artist":    [][]any{{"結束バンド", 54103171}},
		"albumId":   243267741,
		"album":     "ドッペルゲンガー",
		"albumPic":  "https://example.invalid/cover.jpg",
		"bitrate":   320000,
		"duration":  230333,
		"format":    "mp3",
	}
}

// parseContainer builds a container, writes it to a temporary file and parses
// it into an ncm.File.
func parseContainer(t *testing.T, opts ncmtest.Options) *ncm.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.ncm")
	if err := os.WriteFile(path, ncmtest.Build(opts), 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}
	file, err := ncm.Open(path)
	if err != nil {
		t.Fatalf("open container: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := file.Parse(); err != nil {
		t.Fatalf("parse container: %v", err)
	}
	return file
}

func TestHandleAllDecodesEverySection(t *testing.T) {
	cover := payload(300)
	music := ncmtest.MinimalMP3(payload(5000))

	decoded := NewConverter(parseContainer(t, ncmtest.Options{
		Meta:  sampleMeta(),
		Cover: cover,
		Music: music,
	}))
	if err := decoded.HandleAll(); err != nil {
		t.Fatalf("handle all: %v", err)
	}

	if !bytes.HasPrefix(decoded.KeyData, []byte(keyPrefix)) {
		t.Errorf("key data = %q, want the prefix %q", decoded.KeyData, keyPrefix)
	}
	if decoded.MetaData == nil {
		t.Fatal("metadata was not decoded")
	}
	if decoded.MetaData.ID != 2611651882 || decoded.MetaData.Name != "Re:Re:" {
		t.Errorf("metadata = %s", decoded.MetaData)
	}
	if decoded.MetaData.Format != FormatMP3 {
		t.Errorf("format = %q, want %q", decoded.MetaData.Format, FormatMP3)
	}
	if decoded.MetaData.Album == nil {
		t.Fatal("album was not decoded")
	}
	if decoded.MetaData.Album.ID != 243267741 || decoded.MetaData.Album.Name != "ドッペルゲンガー" {
		t.Errorf("album = %s", decoded.MetaData.Album)
	}
	if len(decoded.MetaData.Artists) != 1 || decoded.MetaData.Artists[0].Name != "結束バンド" {
		t.Errorf("artists = %+v", decoded.MetaData.Artists)
	}
	if !bytes.Equal(decoded.Cover.Bytes, cover) {
		t.Error("cover bytes do not match the container contents")
	}
	if !bytes.Equal(decoded.MusicData, music) {
		t.Errorf("decoded audio differs from the original: %d bytes, want %d", len(decoded.MusicData), len(music))
	}
}

// TestHandleMusicKeepsTheTailIntact locks the regression where a short read was
// written out at full block size, padding the result with stale bytes and
// dropping the final chunk.
func TestHandleMusicKeepsTheTailIntact(t *testing.T) {
	sizes := []int{0, 1, 0x8000 - 1, 0x8000, 0x8000 + 1, 2*0x8000 + 1234}

	for _, size := range sizes {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			music := payload(size)
			decoded := NewConverter(parseContainer(t, ncmtest.Options{
				Meta:  sampleMeta(),
				Music: music,
			}))

			if err := decoded.HandleKey(); err != nil {
				t.Fatalf("handle key: %v", err)
			}
			if err := decoded.HandleMusic(); err != nil {
				t.Fatalf("handle music: %v", err)
			}
			if len(decoded.MusicData) != len(music) {
				t.Fatalf("decoded %d bytes, want exactly %d", len(decoded.MusicData), len(music))
			}
			if !bytes.Equal(decoded.MusicData, music) {
				t.Error("decoded audio differs from the original")
			}
		})
	}
}

func TestResolveFormatSniffsAudioWhenMetadataIsAbsent(t *testing.T) {
	tests := []struct {
		name  string
		music []byte
		want  string
	}{
		{name: "flac", music: ncmtest.MinimalFLAC(payload(64)), want: FormatFLAC},
		{name: "id3 tagged mp3", music: ncmtest.MinimalMP3(payload(64)), want: FormatMP3},
		{name: "mp3 frame sync", music: append([]byte{0xff, 0xfb, 0x90, 0x00}, payload(64)...), want: FormatMP3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// No meta section at all, so the format has to come from the audio.
			decoded := NewConverter(parseContainer(t, ncmtest.Options{Music: tt.music}))
			if err := decoded.HandleAll(); err != nil {
				t.Fatalf("handle all: %v", err)
			}
			if decoded.MetaData.Format != tt.want {
				t.Errorf("format = %q, want %q", decoded.MetaData.Format, tt.want)
			}
		})
	}
}

func TestResolveFormatPrefersTheDeclaredFormat(t *testing.T) {
	// The declared format wins over what the audio looks like, and it is
	// lower-cased.
	decoded := NewConverter(parseContainer(t, ncmtest.Options{
		Meta:  map[string]any{"format": "FLAC"},
		Music: ncmtest.MinimalMP3(payload(64)),
	}))
	if err := decoded.HandleAll(); err != nil {
		t.Fatalf("handle all: %v", err)
	}
	if decoded.MetaData.Format != FormatFLAC {
		t.Errorf("format = %q, want %q", decoded.MetaData.Format, FormatFLAC)
	}
}

func TestResolveFormatRejectsUnknownAudio(t *testing.T) {
	decoded := NewConverter(parseContainer(t, ncmtest.Options{Music: []byte("this is not audio")}))

	err := decoded.HandleAll()
	if !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("error = %v, want %v", err, ErrUnknownFormat)
	}
}

func TestHandleMetaRejectsMalformedSection(t *testing.T) {
	decoded := NewConverter(parseContainer(t, ncmtest.Options{
		RawMeta: []byte("this is not a meta section"),
	}))

	err := decoded.HandleMeta()
	if err == nil {
		t.Fatal("handle meta succeeded on a malformed section, want an error")
	}
	if !strings.Contains(err.Error(), metaPrefix) {
		t.Errorf("error = %v, want it to mention the missing prefix", err)
	}
}

func TestHandleMusicRequiresAResolvedKey(t *testing.T) {
	decoded := NewConverter(parseContainer(t, ncmtest.Options{
		Meta:  sampleMeta(),
		Music: payload(128),
	}))

	if err := decoded.HandleMusic(); err == nil {
		t.Fatal("handle music succeeded without a key, want an error")
	}
}

func TestHandleKeyRejectsWrongKeyPrefix(t *testing.T) {
	// A container whose key material does not decrypt to the expected prefix
	// must be reported rather than used.
	data := ncmtest.Build(ncmtest.Options{
		Meta:  sampleMeta(),
		Music: payload(128),
	})
	// Corrupt the first byte of the encrypted key section, right after the
	// 8 byte magic header and the 2 byte padding and the 4 byte length.
	data[10+4] ^= 0xff

	path := filepath.Join(t.TempDir(), "sample.ncm")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}
	file, err := ncm.Open(path)
	if err != nil {
		t.Fatalf("open container: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := file.Parse(); err != nil {
		t.Fatalf("parse container: %v", err)
	}

	decoded := NewConverter(file)
	if err := decoded.HandleKey(); err == nil {
		t.Fatal("handle key succeeded on a corrupted key section, want an error")
	}
}
