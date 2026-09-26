package app

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bogem/id3v2"
	"github.com/chilfish/ncmconverter/internal/ncmtest"
	flacpicture "github.com/go-flac/flacpicture/v2"
	"github.com/go-flac/flacvorbis/v2"
	goflac "github.com/go-flac/go-flac/v2"
)

func TestMain(m *testing.M) {
	// Keep conversion progress out of the test output.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}

// frames returns deterministic audio bytes that are easy to tell apart from tag
// data.
func frames(size int, seed byte) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = byte(i) ^ seed
	}
	return out
}

// container writes a synthetic container into dir and returns its path.
func container(t *testing.T, dir, name string, opts ncmtest.Options) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, ncmtest.Build(opts), 0o644); err != nil {
		t.Fatalf("write container: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// TestRunConvertsMP3EndToEnd converts a container end to end and checks that the
// result is a usable mp3 whose audio is exactly what the container held.
func TestRunConvertsMP3EndToEnd(t *testing.T) {
	audio := frames(20000, 0x5a)
	dir := t.TempDir()
	source := container(t, dir, "song.ncm", ncmtest.Options{
		Meta: map[string]any{
			"musicId":   2611651882,
			"musicName": "Re:Re:",
			"artist":    [][]any{{"結束バンド", 54103171}},
			"albumId":   243267741,
			"album":     "ドッペルゲンガー",
			"format":    "mp3",
		},
		Cover: ncmtest.JPEG(),
		Music: ncmtest.MinimalMP3(audio),
	})

	if err := Run(context.Background(), Options{Inputs: []string{source}, Tag: true, Threads: 2}); err != nil {
		t.Fatalf("run: %v", err)
	}

	dest := filepath.Join(dir, "song.mp3")
	converted := readFile(t, dest)

	if !bytes.HasPrefix(converted, []byte("ID3")) {
		t.Fatalf("converted file does not begin with an ID3v2 tag: % x", converted[:min(len(converted), 8)])
	}
	// The audio has to be byte for byte what the container held: an off by four
	// tail, a dropped final block or stale padding would all break this.
	if !bytes.HasSuffix(converted, audio) {
		t.Errorf("converted file does not end with the original audio: %d bytes written, %d bytes of audio",
			len(converted), len(audio))
	}

	assertMP3Tags(t, dest, "Re:Re:", "結束バンド", "ドッペルゲンガー")
}

func assertMP3Tags(t *testing.T, path, wantTitle, wantArtist, wantAlbum string) {
	t.Helper()
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatalf("open converted mp3: %v", err)
	}
	defer func() { _ = tag.Close() }()

	if got := tag.Title(); got != wantTitle {
		t.Errorf("title = %q, want %q", got, wantTitle)
	}
	if got := tag.Album(); got != wantAlbum {
		t.Errorf("album = %q, want %q", got, wantAlbum)
	}
	// An empty want means the metadata declares no artist, so there is nothing
	// to compare a frame against.
	if wantArtist == "" {
		return
	}
	// Frame identifiers are used literally here so that the assertion does not
	// depend on the library's name mapping.
	//
	// The artist is non-Latin on purpose: the audio arrived carrying an ID3v2.3
	// tag, whose default text encoding cannot represent it.
	artistFrames := tag.GetFrames("TPE1")
	if len(artistFrames) != 1 {
		t.Fatalf("got %d artist frames, want 1", len(artistFrames))
	}
	text, ok := artistFrames[0].(id3v2.TextFrame)
	if !ok {
		t.Fatalf("artist frame is %T, want id3v2.TextFrame", artistFrames[0])
	}
	if text.Text != wantArtist {
		t.Errorf("artist = %q, want %q", text.Text, wantArtist)
	}
	if got := tag.GetFrames("APIC"); len(got) == 0 {
		t.Error("attached picture frame APIC is missing")
	}
}

// TestRunConvertsFLACEndToEnd converts a container end to end and checks that
// the result is a usable FLAC stream whose audio is exactly what the container
// held.
func TestRunConvertsFLACEndToEnd(t *testing.T) {
	stream := ncmtest.MinimalFLAC(frames(9000, 0x33))
	// "fLaC" plus a four byte block header plus the thirty four byte stream info
	// block, so everything from here on is audio.
	audio := stream[42:]

	dir := t.TempDir()
	source := container(t, dir, "song.ncm", ncmtest.Options{
		Meta: map[string]any{
			"musicId":   7,
			"musicName": "Moon Over",
			"artist":    [][]any{{"UNDERCΦDE", 1}},
			"albumId":   8,
			"album":     "Moon Over",
			"format":    "flac",
		},
		Cover: ncmtest.PNG(),
		Music: stream,
	})

	if err := Run(context.Background(), Options{Inputs: []string{source}, Tag: true, Threads: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}

	dest := filepath.Join(dir, "song.flac")
	converted := readFile(t, dest)

	if !bytes.HasPrefix(converted, []byte("fLaC")) {
		t.Fatal("converted file is not a FLAC stream")
	}
	if !bytes.HasSuffix(converted, audio) {
		t.Errorf("converted file does not end with the original audio: %d bytes written, %d bytes of audio",
			len(converted), len(audio))
	}

	assertFLACTags(t, dest, "Moon Over", "UNDERCΦDE")
}

func assertFLACTags(t *testing.T, path, wantTitle, wantArtist string) {
	t.Helper()
	file, err := goflac.ParseFile(path)
	if err != nil {
		t.Fatalf("parse converted flac: %v", err)
	}
	defer func() { _ = file.Close() }()

	if len(file.Meta) == 0 {
		t.Fatal("converted flac has no metadata blocks")
	}
	if file.Meta[0].Type != goflac.StreamInfo {
		t.Errorf("first metadata block is %d, want StreamInfo", file.Meta[0].Type)
	}

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

	// The FLAC specification allows only one comment block.
	if commentBlocks != 1 {
		t.Errorf("converted flac has %d vorbis comment blocks, want exactly 1", commentBlocks)
	}
	if comments == nil {
		t.Fatal("converted flac has no vorbis comment block")
	}

	if got, err := comments.Get(flacvorbis.FIELD_TITLE); err != nil || len(got) != 1 || got[0] != wantTitle {
		t.Errorf("TITLE = %v (err %v), want [%s]", got, err, wantTitle)
	}
	if got, err := comments.Get(flacvorbis.FIELD_ARTIST); err != nil || len(got) != 1 || got[0] != wantArtist {
		t.Errorf("ARTIST = %v (err %v), want [%s]", got, err, wantArtist)
	}
	if len(pictures) != 1 {
		t.Fatalf("converted flac has %d picture blocks, want 1", len(pictures))
	}
	if pictures[0].PictureType != flacpicture.PictureTypeFrontCover {
		t.Errorf("picture type = %d, want front cover", pictures[0].PictureType)
	}
	if pictures[0].MIME != "image/png" {
		t.Errorf("picture MIME = %q, want %q", pictures[0].MIME, "image/png")
	}
}

// TestRunOutputDirectory checks that converted files land in the requested
// directory under the source name with a swapped extension.
func TestRunOutputDirectory(t *testing.T) {
	sourceDir := t.TempDir()
	outputDir := filepath.Join(t.TempDir(), "converted")
	source := container(t, sourceDir, "song.ncm", ncmtest.Options{
		Meta:  map[string]any{"format": "mp3", "musicName": "Track"},
		Music: ncmtest.MinimalMP3(frames(256, 0x11)),
	})

	if err := Run(context.Background(), Options{Inputs: []string{source}, Output: outputDir, Tag: false, Threads: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}

	converted := readFile(t, filepath.Join(outputDir, "song.mp3"))
	if !bytes.HasPrefix(converted, []byte("ID3")) {
		t.Error("converted file does not begin with an ID3v2 tag")
	}
}

// TestRunContinuesAfterAFailure checks that one unusable container does not
// abandon the rest of a batch, and that the failure is still reported.
func TestRunContinuesAfterAFailure(t *testing.T) {
	dir := t.TempDir()
	container(t, dir, "good.ncm", ncmtest.Options{
		Meta:  map[string]any{"format": "mp3", "musicName": "Good"},
		Music: ncmtest.MinimalMP3(frames(512, 0x22)),
	})
	if err := os.WriteFile(filepath.Join(dir, "bad.ncm"), []byte("not an ncm container at all"), 0o644); err != nil {
		t.Fatalf("write bad container: %v", err)
	}

	err := Run(context.Background(), Options{Inputs: []string{dir}, Tag: false, Threads: 2})
	if err == nil {
		t.Fatal("run succeeded even though a container was unusable, want an error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "good.mp3")); statErr != nil {
		t.Errorf("the usable container was not converted: %v", statErr)
	}
}

// TestRunHonoursCancellation checks that a cancelled context stops the batch.
func TestRunHonoursCancellation(t *testing.T) {
	dir := t.TempDir()
	container(t, dir, "song.ncm", ncmtest.Options{
		Meta:  map[string]any{"format": "mp3", "musicName": "Track"},
		Music: ncmtest.MinimalMP3(frames(256, 0x44)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Run(ctx, Options{Inputs: []string{dir}, Tag: false, Threads: 1}); err == nil {
		t.Fatal("run succeeded on a cancelled context, want an error")
	}
}

// TestRunEmbedsTheLyricsSidecar converts containers that have a .lrc next to
// them and checks that the lyrics reach the converted file, one format per
// subtest.
func TestRunEmbedsTheLyricsSidecar(t *testing.T) {
	const lyrics = "[00:01.000] 歌詞"

	t.Run("mp3", func(t *testing.T) {
		dir := t.TempDir()
		source := container(t, dir, "song.ncm", ncmtest.Options{
			Meta:  map[string]any{"format": "mp3", "musicName": "Track"},
			Music: ncmtest.MinimalMP3(frames(512, 0x0b)),
		})
		writeLyricsSidecar(t, source, lyrics)

		if err := Run(context.Background(), Options{Inputs: []string{source}, Tag: true, Threads: 1}); err != nil {
			t.Fatalf("run: %v", err)
		}

		tag, err := id3v2.Open(filepath.Join(dir, "song.mp3"), id3v2.Options{Parse: true})
		if err != nil {
			t.Fatalf("open converted mp3: %v", err)
		}
		defer func() { _ = tag.Close() }()

		uslt, ok := tag.GetLastFrame("USLT").(id3v2.UnsynchronisedLyricsFrame)
		if !ok {
			t.Fatal("converted mp3 has no USLT frame")
		}
		if uslt.Lyrics != lyrics {
			t.Errorf("lyrics = %q, want %q", uslt.Lyrics, lyrics)
		}
	})

	t.Run("flac", func(t *testing.T) {
		dir := t.TempDir()
		source := container(t, dir, "song.ncm", ncmtest.Options{
			Meta:  map[string]any{"format": "flac", "musicName": "Track"},
			Music: ncmtest.MinimalFLAC(frames(512, 0x0c)),
		})
		writeLyricsSidecar(t, source, lyrics)

		if err := Run(context.Background(), Options{Inputs: []string{source}, Tag: true, Threads: 1}); err != nil {
			t.Fatalf("run: %v", err)
		}

		file, err := goflac.ParseFile(filepath.Join(dir, "song.flac"))
		if err != nil {
			t.Fatalf("parse converted flac: %v", err)
		}
		defer func() { _ = file.Close() }()

		var got []string
		for _, block := range file.Meta {
			if block.Type != goflac.VorbisComment {
				continue
			}
			comments, err := flacvorbis.ParseFromMetaDataBlock(*block)
			if err != nil {
				t.Fatalf("parse vorbis comment block: %v", err)
			}
			got, _ = comments.Get("LYRICS")
		}
		if len(got) != 1 || got[0] != lyrics {
			t.Errorf("LYRICS = %v, want [%s]", got, lyrics)
		}
	})
}

// writeLyricsSidecar writes a .lrc file next to a container, named after it.
func writeLyricsSidecar(t *testing.T, source, lyrics string) {
	t.Helper()
	path := strings.TrimSuffix(source, filepath.Ext(source)) + ".lrc"
	if err := os.WriteFile(path, []byte(lyrics), 0o644); err != nil {
		t.Fatalf("write lyrics sidecar: %v", err)
	}
}

// TestRunEmbedsLargeLyrics converts a container whose sidecar is far larger
// than anything NetEase writes, and reads the lyrics back, one format per
// subtest. It pins the boundary where a tag library might silently cap the size
// of what it stores.
func TestRunEmbedsLargeLyrics(t *testing.T) {
	lyrics := largeLyrics()
	if len(lyrics) < 1<<20 {
		t.Fatalf("the fixture is only %d bytes, too small to exercise the boundary", len(lyrics))
	}

	t.Run("mp3", func(t *testing.T) {
		dir := t.TempDir()
		source := container(t, dir, "song.ncm", ncmtest.Options{
			Meta:  map[string]any{"format": "mp3", "musicName": "Track"},
			Music: ncmtest.MinimalMP3(frames(512, 0x1a)),
		})
		writeLyricsSidecar(t, source, lyrics)

		if err := Run(context.Background(), Options{Inputs: []string{source}, Tag: true, Threads: 1}); err != nil {
			t.Fatalf("run: %v", err)
		}

		tag, err := id3v2.Open(filepath.Join(dir, "song.mp3"), id3v2.Options{Parse: true})
		if err != nil {
			t.Fatalf("open converted mp3: %v", err)
		}
		defer func() { _ = tag.Close() }()

		uslt, ok := tag.GetLastFrame("USLT").(id3v2.UnsynchronisedLyricsFrame)
		if !ok {
			t.Fatal("converted mp3 has no USLT frame")
		}
		if len(uslt.Lyrics) != len(lyrics) {
			t.Fatalf("lyrics are %d bytes, want %d", len(uslt.Lyrics), len(lyrics))
		}
		if uslt.Lyrics != lyrics {
			t.Error("the embedded lyrics differ from the sidecar")
		}
	})

	t.Run("flac", func(t *testing.T) {
		dir := t.TempDir()
		source := container(t, dir, "song.ncm", ncmtest.Options{
			Meta:  map[string]any{"format": "flac", "musicName": "Track"},
			Music: ncmtest.MinimalFLAC(frames(512, 0x1b)),
		})
		writeLyricsSidecar(t, source, lyrics)

		if err := Run(context.Background(), Options{Inputs: []string{source}, Tag: true, Threads: 1}); err != nil {
			t.Fatalf("run: %v", err)
		}

		file, err := goflac.ParseFile(filepath.Join(dir, "song.flac"))
		if err != nil {
			t.Fatalf("parse converted flac: %v", err)
		}
		defer func() { _ = file.Close() }()

		var got []string
		for _, block := range file.Meta {
			if block.Type != goflac.VorbisComment {
				continue
			}
			comments, err := flacvorbis.ParseFromMetaDataBlock(*block)
			if err != nil {
				t.Fatalf("parse vorbis comment block: %v", err)
			}
			got, _ = comments.Get("LYRICS")
		}
		if len(got) != 1 {
			t.Fatalf("got %d LYRICS fields, want 1", len(got))
		}
		if len(got[0]) != len(lyrics) {
			t.Fatalf("lyrics are %d bytes, want %d", len(got[0]), len(lyrics))
		}
		if got[0] != lyrics {
			t.Error("the embedded lyrics differ from the sidecar")
		}
	})
}

// TestRunLeavesNoTemporaryFiles converts in place and checks that the directory
// holds nothing but the source, its lyrics sidecar and the result, one format
// per subtest.
//
// Tagging rewrites audio through a temporary file, and a library that fails to
// remove it leaves a leak that a passing conversion would otherwise hide. The
// sidecar makes the lyrics path run as well.
func TestRunLeavesNoTemporaryFiles(t *testing.T) {
	tests := []struct {
		format string
		music  []byte
		ext    string
	}{
		{format: "mp3", music: ncmtest.MinimalMP3(frames(4096, 0x7a)), ext: ".mp3"},
		{format: "flac", music: ncmtest.MinimalFLAC(frames(4096, 0x7b)), ext: ".flac"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			dir := t.TempDir()
			source := container(t, dir, "song.ncm", ncmtest.Options{
				Meta:  map[string]any{"format": tt.format, "musicName": "Track"},
				Cover: ncmtest.JPEG(),
				Music: tt.music,
			})
			writeLyricsSidecar(t, source, "[00:01.000] 歌詞")

			if err := Run(context.Background(), Options{Inputs: []string{source}, Tag: true, Threads: 1}); err != nil {
				t.Fatalf("run: %v", err)
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("read output directory: %v", err)
			}
			got := make([]string, 0, len(entries))
			for _, entry := range entries {
				got = append(got, entry.Name())
			}
			want := []string{"song.ncm", "song.lrc", "song" + tt.ext}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("directory holds %v, want %v", got, want)
			}
		})
	}
}
