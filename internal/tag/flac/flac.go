// Package flac writes Vorbis comment tags and cover art into FLAC files.
package flac

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-flac/flacpicture/v2"
	"github.com/go-flac/flacvorbis/v2"
	"github.com/go-flac/go-flac/v2"
)

// Tag writes tags into a single FLAC file.
type Tag struct {
	path     string
	file     *flac.File
	comments *flacvorbis.MetaDataBlockVorbisComment
	// commentAt is the index of the existing Vorbis comment block, or -1 when
	// the file does not have one yet.
	commentAt int
	closed    bool
}

// New opens the FLAC file at path for tagging.
func New(path string) (*Tag, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open flac file: %w", err)
	}
	// flac.ParseFile leaks the descriptor when parsing fails, because it cannot
	// hand the caller anything to close. Opening the file here keeps that in our
	// hands. The wrapper matters: it is what lets the library find the file
	// again when it rewrites the stream.
	file, err := flac.ParseBytes(flac.NewBufIOWithInner(fd))
	if err != nil {
		_ = fd.Close()
		return nil, fmt.Errorf("parse flac file: %w", err)
	}

	tag := &Tag{path: path, file: file, commentAt: -1}
	for index, block := range file.Meta {
		if block.Type != flac.VorbisComment {
			continue
		}
		comments, err := flacvorbis.ParseFromMetaDataBlock(*block)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("parse vorbis comment block: %w", err)
		}
		// The specification allows only one comment block, but tolerate a file
		// that has more by keeping the last one and rewriting it in place.
		tag.comments = comments
		tag.commentAt = index
	}
	if tag.comments == nil {
		tag.comments = flacvorbis.New()
	}
	return tag, nil
}

// SetCover embeds a cover image.
func (t *Tag) SetCover(cover []byte, mime string) error {
	picture, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "Front cover", cover, mime)
	if err != nil {
		return fmt.Errorf("build picture block: %w", err)
	}
	block := picture.Marshal()
	t.file.Meta = append(t.file.Meta, &block)
	return nil
}

// SetCoverURL stores a link to a remote cover image instead of embedding it.
func (t *Tag) SetCoverURL(coverURL string) error {
	picture := &flacpicture.MetadataBlockPicture{
		PictureType: flacpicture.PictureTypeFrontCover,
		MIME:        flacpicture.MIMEURL,
		Description: "Front cover",
		ImageData:   []byte(coverURL),
	}
	block := picture.Marshal()
	t.file.Meta = append(t.file.Meta, &block)
	return nil
}

// SetTitle sets the title unless the file already declares one.
func (t *Tag) SetTitle(title string) error {
	return t.setOnce(flacvorbis.FIELD_TITLE, title)
}

// SetAlbum sets the album unless the file already declares one.
func (t *Tag) SetAlbum(album string) error {
	return t.setOnce(flacvorbis.FIELD_ALBUM, album)
}

// SetComment sets the description unless the file already declares one.
func (t *Tag) SetComment(comment string) error {
	return t.setOnce(flacvorbis.FIELD_DESCRIPTION, comment)
}

// SetArtists writes one ARTIST entry per name, unless the file already declares
// any.
func (t *Tag) SetArtists(artists []string) error {
	existing, err := t.comments.Get(flacvorbis.FIELD_ARTIST)
	if err != nil {
		return fmt.Errorf("read existing artists: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}
	for _, artist := range artists {
		if err := t.comments.Add(flacvorbis.FIELD_ARTIST, artist); err != nil {
			return fmt.Errorf("add artist %q: %w", artist, err)
		}
	}
	return nil
}

// Save writes the tags back to disk and releases the file.
//
// go-flac rewrites the metadata in place, shifting the audio frames that follow
// it, so the destination may be the file that was opened for parsing.
func (t *Tag) Save() error {
	if t.closed {
		return errors.New("flac file is already closed")
	}

	comments := t.comments.Marshal()
	if t.commentAt >= 0 {
		t.file.Meta[t.commentAt] = &comments
	} else {
		t.file.Meta = append(t.file.Meta, &comments)
	}

	if err := t.file.Save(t.path); err != nil {
		return fmt.Errorf("write tagged flac file: %w", err)
	}
	// Save releases both file handles, so nothing is left to close.
	t.closed = true
	return nil
}

// Close releases the underlying file handle without writing anything. It is
// safe to call more than once.
func (t *Tag) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return t.file.Close()
}

// setOnce adds key=value unless the file already declares that key, so any
// value carried by the source file is preserved.
func (t *Tag) setOnce(key, value string) error {
	existing, err := t.comments.Get(key)
	if err != nil {
		return fmt.Errorf("read %s: %w", key, err)
	}
	if len(existing) > 0 {
		return nil
	}
	if err := t.comments.Add(key, value); err != nil {
		return fmt.Errorf("add %s: %w", key, err)
	}
	return nil
}
