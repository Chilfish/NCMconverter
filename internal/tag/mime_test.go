package tag

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectImageMIME(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "png", data: append(bytes.Clone(pngMagic), 0x01), want: MIMEPNG},
		{name: "jpeg", data: []byte{0xff, 0xd8, 0xff, 0xe0}, want: MIMEJPEG},
		{name: "empty", data: nil, want: MIMEJPEG},
		{name: "truncated png signature", data: pngMagic[:4], want: MIMEJPEG},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectImageMIME(tt.data); got != tt.want {
				t.Errorf("detectImageMIME() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFetchCoverReturnsTheImage(t *testing.T) {
	image := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(image)
	}))
	defer server.Close()

	got, err := fetchCover(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("fetch cover: %v", err)
	}
	if !bytes.Equal(got, image) {
		t.Errorf("fetch cover = %x, want %x", got, image)
	}
}

func TestFetchCoverRejectsErrorResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := fetchCover(context.Background(), server.URL); err == nil {
		t.Fatal("fetch cover succeeded on a 404, want an error")
	}
}

func TestFetchCoverHonoursCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("never read"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := fetchCover(ctx, server.URL); err == nil {
		t.Fatal("fetch cover succeeded on a cancelled context, want an error")
	}
}

func TestFetchCoverRejectsMalformedURL(t *testing.T) {
	if _, err := fetchCover(context.Background(), "://not a url"); err == nil {
		t.Fatal("fetch cover succeeded on a malformed url, want an error")
	}
}
