package app

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// lyricsExtension is the sidecar file that carries a track's lyrics, named
// after the container it belongs to.
const lyricsExtension = ".lrc"

// utf8BOM is the byte order mark an editor may leave at the start of a file.
var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// readLyrics returns the lyrics stored beside a container, or an empty string
// when there is no sidecar for it.
//
// A container without lyrics is the common case, not a failure, so a missing
// sidecar yields no text and no error. The text comes back as a player should
// show it: a leading byte order mark is dropped, and bytes that are not valid
// UTF-8 are decoded as GBK, which is what older tracks use.
func readLyrics(source string) (string, error) {
	path := strings.TrimSuffix(source, filepath.Ext(source)) + lyricsExtension
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read lyrics %s: %w", path, err)
	}

	text, err := decodeLyrics(data)
	if err != nil {
		return "", fmt.Errorf("decode lyrics %s: %w", path, err)
	}
	return text, nil
}

// decodeLyrics turns the bytes of a sidecar into text, dropping a byte order
// mark and falling back to GBK when the content is not UTF-8.
func decodeLyrics(data []byte) (string, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	if utf8.Valid(data) {
		return string(data), nil
	}
	decoded, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), string(data))
	if err != nil {
		return "", err
	}
	return decoded, nil
}
