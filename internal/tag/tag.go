// Package tag writes metadata and cover art into converted audio files.
package tag

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/chilfish/ncmconverter/internal/converter"
	"github.com/chilfish/ncmconverter/internal/tag/flac"
	"github.com/chilfish/ncmconverter/internal/tag/mp3"
)

// MIME types this package assigns to embedded cover art.
const (
	MIMEJPEG = "image/jpeg"
	MIMEPNG  = "image/png"
)

// ErrFormat reports an audio container that cannot be tagged.
var ErrFormat = errors.New("only mp3 and flac can be tagged")

// Tagger writes tags into a single audio file.
//
// Every Set method records a value; Save writes them all to disk and releases
// the file. Close releases the file without saving and is safe to call more
// than once, so callers can defer it alongside Save.
type Tagger interface {
	SetCover(cover []byte, mime string) error
	SetCoverURL(coverURL string) error
	SetTitle(title string) error
	SetAlbum(album string) error
	SetArtists(artists []string) error
	SetComment(comment string) error
	SetLyrics(lyrics string) error
	Save() error
	Close() error
}

// NewTagger opens path for tagging according to the audio format it holds.
func NewTagger(path, format string) (Tagger, error) {
	switch strings.ToLower(format) {
	case converter.FormatMP3:
		return mp3.New(path)
	case converter.FormatFLAC:
		return flac.New(path)
	default:
		return nil, fmt.Errorf("%w: %q", ErrFormat, format)
	}
}

// WriteTo opens the audio file at path and writes the tags described by meta,
// the cover image in cover, and the lyrics in lyrics into it.
//
// When cover is empty and the metadata carries a remote cover URL, the image is
// downloaded instead. A download failure is not fatal: the URL is then stored
// as a link rather than embedded. Empty lyrics are skipped, because having none
// is the common case.
func WriteTo(ctx context.Context, path string, cover []byte, meta *converter.Meta, lyrics string) error {
	if meta == nil {
		return errors.New("write tags: metadata is nil")
	}

	tagger, err := NewTagger(path, meta.Format)
	if err != nil {
		return err
	}
	defer func() {
		if err := tagger.Close(); err != nil {
			slog.Warn("closing tagged file failed", "file", path, "err", err)
		}
	}()

	return WriteTags(ctx, tagger, cover, meta, lyrics)
}

// WriteTags writes the tags described by meta, the lyrics in lyrics, into an
// already-open Tagger and saves them.
func WriteTags(ctx context.Context, tagger Tagger, cover []byte, meta *converter.Meta, lyrics string) error {
	if meta == nil {
		return errors.New("write tags: metadata is nil")
	}

	if err := writeCover(ctx, tagger, cover, meta.Album); err != nil {
		return err
	}
	if meta.Name != "" {
		if err := tagger.SetTitle(meta.Name); err != nil {
			return fmt.Errorf("set title: %w", err)
		}
	}
	if meta.Album != nil && meta.Album.Name != "" {
		if err := tagger.SetAlbum(meta.Album.Name); err != nil {
			return fmt.Errorf("set album: %w", err)
		}
	}
	if artists := artistNames(meta.Artists); len(artists) > 0 {
		if err := tagger.SetArtists(artists); err != nil {
			return fmt.Errorf("set artists: %w", err)
		}
	}
	if meta.Comment != "" {
		if err := tagger.SetComment(meta.Comment); err != nil {
			return fmt.Errorf("set comment: %w", err)
		}
	}
	if lyrics != "" {
		if err := tagger.SetLyrics(lyrics); err != nil {
			return fmt.Errorf("set lyrics: %w", err)
		}
	}

	if err := tagger.Save(); err != nil {
		return fmt.Errorf("save tags: %w", err)
	}
	return nil
}

// writeCover embeds the cover image, falling back to a link when the image has
// to be downloaded and cannot be.
func writeCover(ctx context.Context, tagger Tagger, cover []byte, album *converter.Album) error {
	if len(cover) > 0 {
		if err := tagger.SetCover(cover, detectImageMIME(cover)); err != nil {
			// Cover art is decorative: a tagger that cannot embed it should not
			// stop the rest of the metadata from being written.
			slog.Warn("could not embed cover art", "err", err)
		}
		return nil
	}
	if album == nil || album.CoverURL == "" {
		return nil
	}

	downloaded, err := fetchCover(ctx, album.CoverURL)
	if err != nil {
		slog.Warn("could not download cover art, storing a link instead",
			"url", album.CoverURL, "err", err)
		if err := tagger.SetCoverURL(album.CoverURL); err != nil {
			return fmt.Errorf("set cover url: %w", err)
		}
		return nil
	}
	if err := tagger.SetCover(downloaded, detectImageMIME(downloaded)); err != nil {
		return fmt.Errorf("set cover: %w", err)
	}
	return nil
}

// artistNames collects the non-empty names of the credited artists.
func artistNames(artists []converter.Artist) []string {
	names := make([]string, 0, len(artists))
	for _, artist := range artists {
		if artist.Name != "" {
			names = append(names, artist.Name)
		}
	}
	return names
}
