package app

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// writeSidecar writes a lyrics sidecar next to a source file and returns the
// source path it belongs to.
func writeSidecar(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	source := filepath.Join(dir, name+".ncm")
	if err := os.WriteFile(source, []byte("container"), 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+lyricsExtension), content, 0o644); err != nil {
		t.Fatalf("write lyrics: %v", err)
	}
	return source
}

func TestReadLyricsReturnsTheSiblingSidecar(t *testing.T) {
	dir := t.TempDir()
	source := writeSidecar(t, dir, "song", []byte("[00:01.000] hello"))

	lyrics, err := readLyrics(source)
	if err != nil {
		t.Fatalf("read lyrics: %v", err)
	}
	if lyrics != "[00:01.000] hello" {
		t.Errorf("lyrics = %q, want the sidecar contents", lyrics)
	}
}

// TestReadLyricsWithoutASidecarIsNotAnError locks the common case: most
// containers have no lyrics, and that must not fail a conversion.
func TestReadLyricsWithoutASidecarIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "song.ncm")
	if err := os.WriteFile(source, []byte("container"), 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}

	lyrics, err := readLyrics(source)
	if err != nil {
		t.Fatalf("a missing sidecar should not be an error: %v", err)
	}
	if lyrics != "" {
		t.Errorf("lyrics = %q, want empty", lyrics)
	}
}

func TestReadLyricsStripsTheByteOrderMark(t *testing.T) {
	dir := t.TempDir()
	source := writeSidecar(t, dir, "song", append([]byte{0xef, 0xbb, 0xbf}, []byte("歌词")...))

	lyrics, err := readLyrics(source)
	if err != nil {
		t.Fatalf("read lyrics: %v", err)
	}
	if lyrics != "歌词" {
		t.Errorf("lyrics = %q, want the mark dropped", lyrics)
	}
}

// TestReadLyricsDecodesGBK covers sidecars written before NetEase moved to
// UTF-8, whose bytes are not valid UTF-8 at all.
func TestReadLyricsDecodesGBK(t *testing.T) {
	const want = "作词: 测试"
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(want))
	if err != nil {
		t.Fatalf("encode gbk: %v", err)
	}

	dir := t.TempDir()
	source := writeSidecar(t, dir, "song", encoded)

	lyrics, err := readLyrics(source)
	if err != nil {
		t.Fatalf("read lyrics: %v", err)
	}
	if lyrics != want {
		t.Errorf("lyrics = %q, want %q", lyrics, want)
	}
}

func TestReadLyricsHandlesAnEmptySidecar(t *testing.T) {
	dir := t.TempDir()
	source := writeSidecar(t, dir, "song", nil)

	lyrics, err := readLyrics(source)
	if err != nil {
		t.Fatalf("read lyrics: %v", err)
	}
	if lyrics != "" {
		t.Errorf("lyrics = %q, want empty", lyrics)
	}
}
