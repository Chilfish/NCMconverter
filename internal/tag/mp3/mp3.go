// Package mp3 writes ID3v2 tags into MP3 files.
package mp3

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bogem/id3v2"
)

// mimeURL marks a picture frame that holds a link to an image rather than the
// image itself.
const mimeURL = "-->"

// Tag writes tags into a single MP3 file.
type Tag struct {
	tag    *id3v2.Tag
	closed bool
}

// New opens the MP3 file at path for tagging.
//
// The tag is pinned to ID3v2.4 so that text frames are written as UTF-8. id3v2
// otherwise falls back to ISO-8859-1 for an ID3v2.3 tag, which cannot represent
// any non-Latin title or artist — and the audio inside a container routinely
// arrives carrying an ID3v2.3 tag.
func New(path string) (*Tag, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return nil, fmt.Errorf("open mp3 file: %w", err)
	}
	tag.SetVersion(4)
	return &Tag{tag: tag}, nil
}

// SetCover embeds a cover image.
func (t *Tag) SetCover(cover []byte, mime string) error {
	t.tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF8,
		MimeType:    mime,
		PictureType: id3v2.PTFrontCover,
		Description: "Front cover",
		Picture:     cover,
	})
	return nil
}

// SetCoverURL stores a link to a remote cover image instead of embedding it.
func (t *Tag) SetCoverURL(coverURL string) error {
	t.tag.AddAttachedPicture(id3v2.PictureFrame{
		Encoding:    id3v2.EncodingUTF8,
		MimeType:    mimeURL,
		PictureType: id3v2.PTFrontCover,
		Description: "Front cover",
		Picture:     []byte(coverURL),
	})
	return nil
}

// SetTitle sets the title unless the file already declares one.
func (t *Tag) SetTitle(title string) error {
	if t.tag.Title() == "" {
		t.tag.SetTitle(title)
	}
	return nil
}

// SetAlbum sets the album unless the file already declares one.
func (t *Tag) SetAlbum(album string) error {
	if t.tag.Album() == "" {
		t.tag.SetAlbum(album)
	}
	return nil
}

// SetArtists writes every name into a single artist frame.
//
// id3v2.SetArtist replaces the frame, so the names are joined rather than set
// one after another.
func (t *Tag) SetArtists(artists []string) error {
	if len(t.tag.GetFrames(t.tag.CommonID("Artist"))) > 0 {
		return nil
	}
	t.tag.SetArtist(strings.Join(artists, "; "))
	return nil
}

// SetComment sets a comment frame unless the file already declares one.
func (t *Tag) SetComment(comment string) error {
	if len(t.tag.GetFrames(t.tag.CommonID("Comments"))) > 0 {
		return nil
	}
	t.tag.AddCommentFrame(id3v2.CommentFrame{
		Encoding:    id3v2.EncodingUTF8,
		Language:    "XXX",
		Description: "",
		Text:        comment,
	})
	return nil
}

// Save writes the tags back to disk and releases the file.
func (t *Tag) Save() error {
	if t.closed {
		return errors.New("mp3 file is already closed")
	}
	saveErr := t.tag.Save()
	return errors.Join(saveErr, t.Close())
}

// Close releases the underlying file handle without writing anything. It is
// safe to call more than once.
func (t *Tag) Close() error {
	if t.closed {
		return nil
	}
	t.closed = true
	return t.tag.Close()
}
