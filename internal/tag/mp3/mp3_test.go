package mp3

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bogem/id3v2"
	"github.com/chilfish/NCMconverter/internal/ncmtest"
)

// writeMinimalMP3 writes a minimal mp3 stream into a temporary file and returns
// its path together with the audio bytes that follow the empty ID3v2 tag.
func writeMinimalMP3(t *testing.T) (string, []byte) {
	t.Helper()
	audio := make([]byte, 1024)
	for i := range audio {
		audio[i] = byte(i) ^ 0x21
	}

	path := filepath.Join(t.TempDir(), "song.mp3")
	if err := os.WriteFile(path, ncmtest.MinimalMP3(audio), 0o644); err != nil {
		t.Fatalf("write mp3: %v", err)
	}
	return path, audio
}

func TestSaveWritesTagsAndKeepsTheAudio(t *testing.T) {
	path, audio := writeMinimalMP3(t)

	tag, err := New(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Non-Latin text is the point of this test: the audio carries an ID3v2.3
	// tag, and a tagger that follows that version's default text encoding cannot
	// represent it at all.
	if err := tag.SetTitle("結束バンド"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := tag.SetAlbum("ドッペルゲンガー"); err != nil {
		t.Fatalf("set album: %v", err)
	}
	if err := tag.SetArtists([]string{"結束バンド", "A"}); err != nil {
		t.Fatalf("set artists: %v", err)
	}
	if err := tag.SetComment("a comment"); err != nil {
		t.Fatalf("set comment: %v", err)
	}
	if err := tag.SetCover([]byte{0xff, 0xd8, 0xff, 0xe0}, "image/jpeg"); err != nil {
		t.Fatalf("set cover: %v", err)
	}
	if err := tag.SetLyrics("[00:01.000] 歌詞"); err != nil {
		t.Fatalf("set lyrics: %v", err)
	}
	if err := tag.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read converted mp3: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("ID3")) {
		t.Error("converted file does not begin with an ID3v2 tag")
	}
	if !bytes.HasSuffix(data, audio) {
		t.Error("the audio was not preserved")
	}

	reopened, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("reopen converted mp3: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	if got := reopened.Title(); got != "結束バンド" {
		t.Errorf("title = %q, want %q", got, "結束バンド")
	}
	if got := reopened.Album(); got != "ドッペルゲンガー" {
		t.Errorf("album = %q, want %q", got, "ドッペルゲンガー")
	}

	// Both artists have to survive. Setting them one after another used to
	// leave only the last one, because SetArtist replaces the frame.
	artistFrames := reopened.GetFrames("TPE1")
	if len(artistFrames) != 1 {
		t.Fatalf("got %d artist frames, want 1", len(artistFrames))
	}
	text, ok := artistFrames[0].(id3v2.TextFrame)
	if !ok {
		t.Fatalf("artist frame is %T, want id3v2.TextFrame", artistFrames[0])
	}
	if text.Text != "結束バンド; A" {
		t.Errorf("artist text = %q, want %q", text.Text, "結束バンド; A")
	}
	if len(reopened.GetFrames("APIC")) == 0 {
		t.Error("attached picture frame APIC is missing")
	}
	if len(reopened.GetFrames("COMM")) == 0 {
		t.Error("comment frame COMM is missing")
	}

	// The lyrics frame is checked by its identifier so that the assertion does
	// not depend on the library's name mapping.
	lyricsFrames := reopened.GetFrames("USLT")
	if len(lyricsFrames) != 1 {
		t.Fatalf("got %d lyrics frames, want 1", len(lyricsFrames))
	}
	uslt, ok := lyricsFrames[0].(id3v2.UnsynchronisedLyricsFrame)
	if !ok {
		t.Fatalf("lyrics frame is %T, want id3v2.UnsynchronisedLyricsFrame", lyricsFrames[0])
	}
	if uslt.Lyrics != "[00:01.000] 歌詞" {
		t.Errorf("lyrics = %q, want %q", uslt.Lyrics, "[00:01.000] 歌詞")
	}
}

// TestSaveTwiceKeepsOneCommentBlock locks the regression where the error from
// saving was discarded, and the file closed twice.
func TestSaveTwiceFails(t *testing.T) {
	path, _ := writeMinimalMP3(t)

	tag, err := New(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := tag.Save(); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := tag.Save(); err == nil {
		t.Fatal("second save succeeded, want an error")
	}
}

func TestSettersPreserveExistingValues(t *testing.T) {
	path, _ := writeMinimalMP3(t)

	first, err := New(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := first.SetTitle("Original"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := first.SetLyrics("original lyrics"); err != nil {
		t.Fatalf("set lyrics: %v", err)
	}
	if err := first.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	second, err := New(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := second.SetTitle("Replacement"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := second.SetLyrics("replacement lyrics"); err != nil {
		t.Fatalf("set lyrics: %v", err)
	}
	if err := second.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	reopened, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()

	if got := reopened.Title(); got != "Original" {
		t.Errorf("title = %q, want the original value to be preserved", got)
	}
	uslt, ok := reopened.GetLastFrame("USLT").(id3v2.UnsynchronisedLyricsFrame)
	if !ok {
		t.Fatal("lyrics frame USLT is missing")
	}
	if uslt.Lyrics != "original lyrics" {
		t.Errorf("lyrics = %q, want the original value to be preserved", uslt.Lyrics)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	path, _ := writeMinimalMP3(t)

	tag, err := New(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := tag.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := tag.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
