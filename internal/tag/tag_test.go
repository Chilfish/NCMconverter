package tag

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/chilfish/NCMconverter/internal/converter"
)

// fakeTagger records the calls made to it so that tests can tell a field that
// was written from one that was skipped.
type fakeTagger struct {
	calls     []string
	cover     []byte
	coverMIME string
	coverURL  string
	title     string
	album     string
	artists   []string
	comment   string
	lyrics    string
	saved     bool
	err       error
	// coverErr fails only SetCover, so the cover art path can be exercised on
	// its own.
	coverErr error
}

func (f *fakeTagger) record(name string) { f.calls = append(f.calls, name) }

func (f *fakeTagger) called(name string) bool { return slices.Contains(f.calls, name) }

func (f *fakeTagger) SetCover(cover []byte, mime string) error {
	f.record("SetCover")
	f.cover, f.coverMIME = cover, mime
	if f.coverErr != nil {
		return f.coverErr
	}
	return f.err
}

func (f *fakeTagger) SetCoverURL(coverURL string) error {
	f.record("SetCoverURL")
	f.coverURL = coverURL
	return f.err
}

func (f *fakeTagger) SetTitle(title string) error {
	f.record("SetTitle")
	f.title = title
	return f.err
}

func (f *fakeTagger) SetAlbum(album string) error {
	f.record("SetAlbum")
	f.album = album
	return f.err
}

func (f *fakeTagger) SetArtists(artists []string) error {
	f.record("SetArtists")
	f.artists = artists
	return f.err
}

func (f *fakeTagger) SetComment(comment string) error {
	f.record("SetComment")
	f.comment = comment
	return f.err
}

func (f *fakeTagger) SetLyrics(lyrics string) error {
	f.record("SetLyrics")
	f.lyrics = lyrics
	return f.err
}

func (f *fakeTagger) Save() error {
	f.record("Save")
	f.saved = true
	return f.err
}

func (f *fakeTagger) Close() error {
	f.record("Close")
	return nil
}

func TestWriteTagsWritesEveryField(t *testing.T) {
	tagger := &fakeTagger{}
	meta := &converter.Meta{
		Name:    "Re:Re:",
		Format:  converter.FormatMP3,
		Comment: "raw meta section",
		// An artist without a name must not reach the file.
		Artists: []converter.Artist{{Name: "結束バンド", ID: 1}, {Name: ""}},
		Album:   &converter.Album{Name: "ドッペルゲンガー"},
	}

	if err := WriteTags(context.Background(), tagger, []byte{0xff, 0xd8, 0xff}, meta, "the lyrics"); err != nil {
		t.Fatalf("write tags: %v", err)
	}

	if tagger.title != "Re:Re:" {
		t.Errorf("title = %q, want %q", tagger.title, "Re:Re:")
	}
	if tagger.album != "ドッペルゲンガー" {
		t.Errorf("album = %q, want %q", tagger.album, "ドッペルゲンガー")
	}
	if len(tagger.artists) != 1 || tagger.artists[0] != "結束バンド" {
		t.Errorf("artists = %v, want only the named artist", tagger.artists)
	}
	if tagger.comment != "raw meta section" {
		t.Errorf("comment = %q", tagger.comment)
	}
	if tagger.coverMIME != MIMEJPEG {
		t.Errorf("cover mime = %q, want %q", tagger.coverMIME, MIMEJPEG)
	}
	if tagger.lyrics != "the lyrics" {
		t.Errorf("lyrics = %q, want %q", tagger.lyrics, "the lyrics")
	}
	if !tagger.saved {
		t.Error("tags were not saved")
	}
}

// TestWriteTagsSkipsAbsentFields locks the regressions that wrote an empty
// title, wrote the track name as the album, and embedded an empty cover.
func TestWriteTagsSkipsAbsentFields(t *testing.T) {
	tagger := &fakeTagger{}
	meta := &converter.Meta{Format: converter.FormatMP3}

	if err := WriteTags(context.Background(), tagger, nil, meta, ""); err != nil {
		t.Fatalf("write tags: %v", err)
	}

	if tagger.called("SetTitle") {
		t.Error("wrote a title even though the metadata has none")
	}
	if tagger.called("SetAlbum") {
		t.Error("wrote an album even though the metadata has none")
	}
	if tagger.called("SetArtists") {
		t.Error("wrote artists even though the metadata has none")
	}
	if tagger.called("SetComment") {
		t.Error("wrote a comment even though the metadata has none")
	}
	if tagger.called("SetCover") || tagger.called("SetCoverURL") {
		t.Error("wrote cover art even though there is none")
	}
	if tagger.called("SetLyrics") {
		t.Error("wrote lyrics even though there are none")
	}
	if !tagger.saved {
		t.Error("tags were not saved")
	}
}

// TestWriteTagsWithoutAlbumDoesNotPanic locks the regression where a container
// without a meta section left the album nil and tagging dereferenced it.
func TestWriteTagsWithoutAlbumDoesNotPanic(t *testing.T) {
	tagger := &fakeTagger{}
	meta := &converter.Meta{Name: "Track", Format: converter.FormatMP3}

	if err := WriteTags(context.Background(), tagger, nil, meta, ""); err != nil {
		t.Fatalf("write tags: %v", err)
	}
	if tagger.title != "Track" {
		t.Errorf("title = %q, want %q", tagger.title, "Track")
	}
}

func TestWriteTagsDownloadsCoverAndDetectsItsType(t *testing.T) {
	image := append(bytes.Clone(pngMagic), 0x01, 0x02)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(image)
	}))
	defer server.Close()

	tagger := &fakeTagger{}
	meta := &converter.Meta{Album: &converter.Album{CoverURL: server.URL}}

	if err := WriteTags(context.Background(), tagger, nil, meta, ""); err != nil {
		t.Fatalf("write tags: %v", err)
	}
	if !bytes.Equal(tagger.cover, image) {
		t.Error("downloaded cover was not embedded")
	}
	if tagger.coverMIME != MIMEPNG {
		t.Errorf("cover mime = %q, want %q", tagger.coverMIME, MIMEPNG)
	}
	if tagger.called("SetCoverURL") {
		t.Error("stored a link even though the download succeeded")
	}
}

// TestWriteTagsFallsBackToALink covers a cover URL that cannot be fetched.
func TestWriteTagsFallsBackToALink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tagger := &fakeTagger{}
	meta := &converter.Meta{Album: &converter.Album{CoverURL: server.URL}}

	if err := WriteTags(context.Background(), tagger, nil, meta, ""); err != nil {
		t.Fatalf("write tags: %v", err)
	}
	if tagger.coverURL != server.URL {
		t.Errorf("cover url = %q, want %q", tagger.coverURL, server.URL)
	}
	if tagger.called("SetCover") {
		t.Error("embedded a cover even though the download failed")
	}
}

func TestWriteTagsPropagatesTaggerErrors(t *testing.T) {
	sentinel := errors.New("tagger exploded")
	tagger := &fakeTagger{err: sentinel}
	meta := &converter.Meta{Name: "Track", Format: converter.FormatMP3}

	err := WriteTags(context.Background(), tagger, nil, meta, "")
	if !errors.Is(err, sentinel) {
		t.Fatalf("write tags error = %v, want it to wrap %v", err, sentinel)
	}
}

func TestWriteTagsRejectsNilMetadata(t *testing.T) {
	if err := WriteTags(context.Background(), &fakeTagger{}, nil, nil, ""); err == nil {
		t.Fatal("write tags succeeded without metadata, want an error")
	}
}

// TestWriteTagsKeepsGoingWhenTheCoverCannotBeEmbedded checks that decorative
// cover art does not stop the rest of the metadata from being written.
func TestWriteTagsKeepsGoingWhenTheCoverCannotBeEmbedded(t *testing.T) {
	tagger := &fakeTagger{coverErr: errors.New("unsupported image")}
	meta := &converter.Meta{Name: "Track", Format: converter.FormatMP3}

	if err := WriteTags(context.Background(), tagger, []byte{0xff, 0xd8}, meta, ""); err != nil {
		t.Fatalf("a cover that cannot be embedded should not fail the write: %v", err)
	}
	if tagger.title != "Track" {
		t.Errorf("title = %q, want the title to be written anyway", tagger.title)
	}
	if !tagger.saved {
		t.Error("tags were not saved")
	}
}

func TestNewTaggerRejectsUnsupportedFormats(t *testing.T) {
	for _, format := range []string{"wav", "", "ogg"} {
		if _, err := NewTagger("unused", format); !errors.Is(err, ErrFormat) {
			t.Errorf("NewTagger(%q) error = %v, want %v", format, err, ErrFormat)
		}
	}
}
