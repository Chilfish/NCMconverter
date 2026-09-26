package flac

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chilfish/NCMconverter/internal/ncmtest"
	flacpicture "github.com/go-flac/flacpicture/v2"
	"github.com/go-flac/flacvorbis/v2"
	goflac "github.com/go-flac/go-flac/v2"
)

// writeMinimalFLAC writes a minimal FLAC stream into a temporary file and
// returns its path together with the audio bytes that follow the stream info
// block.
func writeMinimalFLAC(t *testing.T) (string, []byte) {
	t.Helper()
	audio := make([]byte, 1024)
	for i := range audio {
		audio[i] = byte(i) ^ 0x3c
	}
	stream := ncmtest.MinimalFLAC(audio)

	path := filepath.Join(t.TempDir(), "song.flac")
	if err := os.WriteFile(path, stream, 0o644); err != nil {
		t.Fatalf("write flac: %v", err)
	}
	// "fLaC" plus a four byte block header plus the thirty four byte stream
	// info block.
	return path, stream[42:]
}

// open opens the FLAC file at path for tagging and makes sure the descriptor is
// released even when the test stops early.
func open(t *testing.T, path string) *Tag {
	t.Helper()
	tag, err := New(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = tag.Close() })
	return tag
}

// inspect reports the single vorbis comment block, how many comment blocks
// there are, and the picture blocks of a FLAC file.
func inspect(t *testing.T, path string) (*flacvorbis.MetaDataBlockVorbisComment, int, []*flacpicture.MetadataBlockPicture) {
	t.Helper()
	file, err := goflac.ParseFile(path)
	if err != nil {
		t.Fatalf("parse flac: %v", err)
	}
	defer func() { _ = file.Close() }()

	var comments *flacvorbis.MetaDataBlockVorbisComment
	commentBlocks := 0
	var pictures []*flacpicture.MetadataBlockPicture
	for _, block := range file.Meta {
		switch block.Type {
		case goflac.VorbisComment:
			commentBlocks++
			comments, err = flacvorbis.ParseFromMetaDataBlock(*block)
			if err != nil {
				t.Fatalf("parse vorbis comment block: %v", err)
			}
		case goflac.Picture:
			picture, err := flacpicture.ParseFromMetaDataBlock(*block)
			if err != nil {
				t.Fatalf("parse picture block: %v", err)
			}
			pictures = append(pictures, picture)
		}
	}
	return comments, commentBlocks, pictures
}

func TestSaveWritesTagsAndKeepsTheAudio(t *testing.T) {
	path, audio := writeMinimalFLAC(t)

	tag := open(t, path)
	if err := tag.SetTitle("Moon Over"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := tag.SetAlbum("Album"); err != nil {
		t.Fatalf("set album: %v", err)
	}
	if err := tag.SetArtists([]string{"A", "B"}); err != nil {
		t.Fatalf("set artists: %v", err)
	}
	if err := tag.SetCover(ncmtest.JPEG(), "image/jpeg"); err != nil {
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
		t.Fatalf("read converted flac: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("fLaC")) {
		t.Error("converted file is not a FLAC stream")
	}
	if !bytes.HasSuffix(data, audio) {
		t.Error("the audio frames were not preserved")
	}

	comments, commentBlocks, pictures := inspect(t, path)
	if commentBlocks != 1 {
		t.Errorf("file has %d vorbis comment blocks, want exactly 1", commentBlocks)
	}
	if comments == nil {
		t.Fatal("file has no vorbis comment block")
	}
	if title, _ := comments.Get(flacvorbis.FIELD_TITLE); !slices.Equal(title, []string{"Moon Over"}) {
		t.Errorf("TITLE = %v, want [Moon Over]", title)
	}
	if artists, _ := comments.Get(flacvorbis.FIELD_ARTIST); !slices.Equal(artists, []string{"A", "B"}) {
		t.Errorf("ARTIST = %v, want [A B]", artists)
	}
	if lyrics, _ := comments.Get(lyricsField); !slices.Equal(lyrics, []string{"[00:01.000] 歌詞"}) {
		t.Errorf("LYRICS = %v, want the lyrics back", lyrics)
	}
	if len(pictures) != 1 {
		t.Fatalf("file has %d picture blocks, want 1", len(pictures))
	}
	if pictures[0].MIME != "image/jpeg" {
		t.Errorf("picture MIME = %q, want %q", pictures[0].MIME, "image/jpeg")
	}
}

// TestSaveTwiceKeepsOneCommentBlock locks the regression where every save
// appended another comment block, which the FLAC specification does not allow.
func TestSaveTwiceKeepsOneCommentBlock(t *testing.T) {
	path, _ := writeMinimalFLAC(t)

	for _, title := range []string{"Original", "Replacement"} {
		tag := open(t, path)
		if err := tag.SetTitle(title); err != nil {
			t.Fatalf("set title: %v", err)
		}
		if err := tag.SetLyrics("original lyrics"); err != nil {
			t.Fatalf("set lyrics: %v", err)
		}
		if err := tag.Save(); err != nil {
			t.Fatalf("save %q: %v", title, err)
		}
	}

	comments, commentBlocks, _ := inspect(t, path)
	if commentBlocks != 1 {
		t.Errorf("after two saves the file has %d vorbis comment blocks, want exactly 1", commentBlocks)
	}
	title, _ := comments.Get(flacvorbis.FIELD_TITLE)
	if !slices.Equal(title, []string{"Original"}) {
		t.Errorf("TITLE = %v, want the original value to be preserved", title)
	}
	lyrics, _ := comments.Get(lyricsField)
	if !slices.Equal(lyrics, []string{"original lyrics"}) {
		t.Errorf("LYRICS = %v, want the original value to be preserved", lyrics)
	}
}

func TestSetCoverURLStoresALink(t *testing.T) {
	path, _ := writeMinimalFLAC(t)
	const coverURL = "https://example.invalid/cover.jpg"

	tag := open(t, path)
	if err := tag.SetCoverURL(coverURL); err != nil {
		t.Fatalf("set cover url: %v", err)
	}
	if err := tag.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	_, _, pictures := inspect(t, path)
	if len(pictures) != 1 {
		t.Fatalf("file has %d picture blocks, want 1", len(pictures))
	}
	if pictures[0].MIME != flacpicture.MIMEURL {
		t.Errorf("picture MIME = %q, want %q", pictures[0].MIME, flacpicture.MIMEURL)
	}
	if string(pictures[0].ImageData) != coverURL {
		t.Errorf("picture data = %q, want the cover url", pictures[0].ImageData)
	}
}

func TestCloseIsIdempotentAndSaveAfterCloseFails(t *testing.T) {
	path, _ := writeMinimalFLAC(t)

	tag := open(t, path)
	if err := tag.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := tag.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if err := tag.Save(); err == nil {
		t.Fatal("save after close succeeded, want an error")
	}
}

func TestNewRejectsNonFLACFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not.flac")
	if err := os.WriteFile(path, []byte("this is not a flac stream"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("New succeeded on a non-FLAC file, want an error")
	}
}
