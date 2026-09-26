package tag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// pngMagic is the eight byte signature that begins a PNG image. Cover art that
// is not a PNG is assumed to be a JPEG.
var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

// maxCoverSize caps how many bytes of a remote cover image are read.
const maxCoverSize = 16 << 20

// coverHTTPClient fetches remote cover art.
var coverHTTPClient = &http.Client{Timeout: 30 * time.Second}

// detectImageMIME reports the MIME type of an image from its leading bytes.
func detectImageMIME(data []byte) string {
	if isPNG(data) {
		return MIMEPNG
	}
	return MIMEJPEG
}

// isPNG reports whether data begins with the PNG signature.
func isPNG(data []byte) bool {
	return bytes.HasPrefix(data, pngMagic)
}

// fetchCover downloads the image at rawURL.
func fetchCover(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build cover request: %w", err)
	}

	res, err := coverHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download cover: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download cover: remote returned %s", res.Status)
	}

	data, err := io.ReadAll(io.LimitReader(res.Body, maxCoverSize))
	if err != nil {
		return nil, fmt.Errorf("read cover body: %w", err)
	}
	return data, nil
}
