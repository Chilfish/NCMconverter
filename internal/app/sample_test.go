package app

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/bogem/id3v2"
	"github.com/chilfish/NCMconverter/internal/converter"
	"github.com/chilfish/NCMconverter/internal/ncm"
)

// repositorySamples returns the containers under testdata/.
//
// A checkout without a testdata/ directory skips, because the fixtures live
// outside the module. A directory that is there but holds no container fails:
// the end-to-end checks below would otherwise pass without doing anything.
func repositorySamples(t *testing.T) []string {
	t.Helper()

	dir := filepath.Join("..", "..", "testdata")
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("no %s directory", dir)
	}

	samples, err := filepath.Glob(filepath.Join(dir, "*.ncm"))
	if err != nil {
		t.Fatalf("find samples in %s: %v", dir, err)
	}
	if len(samples) == 0 {
		t.Fatalf("%s holds no container, so these end-to-end checks would not run", dir)
	}
	return samples
}

// TestRunConvertsTheRepositorySample converts the sample container that ships
// with the repository, and checks that tagging left the audio untouched.
func TestRunConvertsTheRepositorySample(t *testing.T) {
	for _, sample := range repositorySamples(t) {
		t.Run(filepath.Base(sample), func(t *testing.T) {
			dir := t.TempDir()
			if err := Run(context.Background(), Options{
				Inputs:  []string{sample},
				Output:  dir,
				Tag:     true,
				Threads: 1,
			}); err != nil {
				t.Fatalf("run: %v", err)
			}

			converted := onlyFile(t, dir)
			assertConvertedAudioIsIntact(t, sample, converted)
		})
	}
}

// onlyFile returns the single file that a conversion produced.
func onlyFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read output directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("conversion produced %d files, want 1", len(entries))
	}
	return filepath.Join(dir, entries[0].Name())
}

// assertConvertedAudioIsIntact compares the audio in a converted file against
// the audio the container holds.
//
// Tagging rewrites the leading ID3v2 tag, so the regions either side of it are
// what have to match: any truncated, duplicated or padded frame would show up
// as a difference in length or content.
func assertConvertedAudioIsIntact(t *testing.T, source, converted string) {
	t.Helper()

	file, err := ncm.Open(source)
	if err != nil {
		t.Fatalf("open container: %v", err)
	}
	defer func() { _ = file.Close() }()
	if err := file.Parse(); err != nil {
		t.Fatalf("parse container: %v", err)
	}

	decoded := converter.NewConverter(file)
	if err := decoded.HandleAll(); err != nil {
		t.Fatalf("decode container: %v", err)
	}

	data := readFile(t, converted)

	switch decoded.MetaData.Format {
	case converter.FormatMP3:
		if !bytes.HasPrefix(data, []byte("ID3")) {
			t.Fatal("converted mp3 does not begin with an ID3v2 tag")
		}
		want := decoded.MusicData[id3v2TagSize(t, decoded.MusicData):]
		got := data[id3v2TagSize(t, data):]
		if !bytes.Equal(got, want) {
			t.Errorf("converted audio differs from the container contents: %d bytes, want %d", len(got), len(want))
		}
		assertMP3Tags(t, converted, decoded.MetaData.Name, decoded.MetaData.Artists[0].Name, decoded.MetaData.Album.Name)
	case converter.FormatFLAC:
		if !bytes.HasPrefix(data, []byte("fLaC")) {
			t.Fatal("converted file is not a FLAC stream")
		}
		// FLAC stores frames after its metadata blocks, so the audio is a
		// suffix of the decoded stream rather than a region at a fixed offset.
		frames := decoded.MusicData[flacHeaderSize(t, decoded.MusicData):]
		if !bytes.HasSuffix(data, frames) {
			t.Errorf("converted flac does not end with the container's audio frames")
		}
	default:
		t.Fatalf("unexpected audio format %q", decoded.MetaData.Format)
	}
}

// id3v2TagSize returns how many leading bytes an ID3v2 tag occupies.
func id3v2TagSize(t *testing.T, data []byte) int {
	t.Helper()
	if len(data) < 10 || string(data[:3]) != "ID3" {
		t.Fatalf("data does not begin with an ID3v2 tag")
	}
	// The size is stored as four synchronisation safe bytes.
	size := int(data[6])<<21 | int(data[7])<<14 | int(data[8])<<7 | int(data[9])
	total := 10 + size
	if data[5]&0x10 != 0 {
		total += 10 // a footer follows the frames
	}
	if total > len(data) {
		t.Fatalf("ID3v2 tag claims %d bytes of a %d byte file", total, len(data))
	}
	return total
}

// flacHeaderSize returns how many leading bytes the FLAC marker and metadata
// blocks occupy.
func flacHeaderSize(t *testing.T, data []byte) int {
	t.Helper()
	if len(data) < 4 || string(data[:4]) != "fLaC" {
		t.Fatalf("data does not begin with a FLAC stream")
	}
	offset := 4
	for {
		if offset+4 > len(data) {
			t.Fatal("FLAC metadata blocks are truncated")
		}
		last := data[offset]&0x80 != 0
		length := int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		offset += 4 + length
		if last {
			break
		}
	}
	return offset
}

// TestRunTagsTheConvertedFileFromTheRepositorySample checks the metadata that
// ends up in a converted sample, including the non-Latin fields.
func TestRunTagsTheConvertedFileFromTheRepositorySample(t *testing.T) {
	samples := repositorySamples(t)

	dir := t.TempDir()
	if err := Run(context.Background(), Options{Inputs: samples[:1], Output: dir, Tag: true, Threads: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}

	converted := onlyFile(t, dir)
	if filepath.Ext(converted) != ".mp3" {
		t.Skipf("the sample converts to %s, whose tags this test does not read", filepath.Ext(converted))
	}

	tag, err := id3v2.Open(converted, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("open converted mp3: %v", err)
	}
	defer func() { _ = tag.Close() }()

	if tag.Title() == "" {
		t.Error("converted file has no title")
	}
	if tag.Album() == "" {
		t.Error("converted file has no album")
	}
	if len(tag.GetFrames("TPE1")) == 0 {
		t.Error("converted file has no artist")
	}
	if len(tag.GetFrames("APIC")) == 0 {
		t.Error("converted file has no cover art")
	}
}
