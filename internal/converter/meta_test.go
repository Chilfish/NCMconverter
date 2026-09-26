package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInt64AcceptsNumbersAndStrings(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Int64
		wantErr bool
	}{
		{name: "json number", raw: `{"id":2611651882}`, want: 2611651882},
		{name: "quoted number", raw: `{"id":"2611651882"}`, want: 2611651882},
		{name: "float", raw: `{"id":123.75}`, want: 123},
		{name: "quoted float", raw: `{"id":"123.75"}`, want: 123},
		{name: "negative", raw: `{"id":-5}`, want: -5},
		{name: "null", raw: `{"id":null}`, want: 0},
		{name: "empty string", raw: `{"id":""}`, want: 0},
		{name: "not a number", raw: `{"id":"abc"}`, wantErr: true},
		{name: "object", raw: `{"id":{}}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var decoded struct {
				ID Int64 `json:"id"`
			}
			err := json.Unmarshal([]byte(tt.raw), &decoded)
			if tt.wantErr {
				if err == nil {
					t.Fatal("unmarshal succeeded, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.ID != tt.want {
				t.Errorf("id = %d, want %d", decoded.ID, tt.want)
			}
		})
	}
}

func TestInt64String(t *testing.T) {
	if got := Int64(2611651882).String(); got != "2611651882" {
		t.Errorf("String() = %q, want %q", got, "2611651882")
	}
}

// TestMetaDecodesIdentifiersEncodedAsStrings locks the regression where a nil
// map made the string-identifier fallback silently unreachable, so quoted ids
// decoded to zero.
func TestMetaDecodesIdentifiersEncodedAsStrings(t *testing.T) {
	// Quoted ids, a quoted duration, and an artist id given as a string.
	payload := `{
		"musicId": "2611651882",
		"musicName": "Re:Re:",
		"artist": [["結束バンド", "54103171"]],
		"albumId": "243267741",
		"album": "ドッペルゲンガー",
		"albumPic": "https://example.invalid/cover.jpg",
		"bitrate": 320000,
		"duration": "230333",
		"format": "mp3"
	}`

	var meta Meta
	if err := json.Unmarshal([]byte(payload), &meta); err != nil {
		t.Fatalf("decode track metadata: %v", err)
	}
	var album Album
	if err := json.Unmarshal([]byte(payload), &album); err != nil {
		t.Fatalf("decode album metadata: %v", err)
	}

	if meta.ID != 2611651882 {
		t.Errorf("musicId = %d, want 2611651882", meta.ID)
	}
	if meta.Name != "Re:Re:" {
		t.Errorf("musicName = %q, want %q", meta.Name, "Re:Re:")
	}
	if meta.Duration != 230333 {
		t.Errorf("duration = %d, want 230333", meta.Duration)
	}
	if meta.BitRate != 320000 {
		t.Errorf("bitrate = %d, want 320000", meta.BitRate)
	}
	if meta.Format != "mp3" {
		t.Errorf("format = %q, want %q", meta.Format, "mp3")
	}
	if album.ID != 243267741 {
		t.Errorf("albumId = %d, want 243267741", album.ID)
	}
	if album.Name != "ドッペルゲンガー" {
		t.Errorf("album = %q, want %q", album.Name, "ドッペルゲンガー")
	}
	if album.CoverURL != "https://example.invalid/cover.jpg" {
		t.Errorf("albumPic = %q", album.CoverURL)
	}
	if len(meta.Artists) != 1 {
		t.Fatalf("decoded %d artists, want 1", len(meta.Artists))
	}
	if meta.Artists[0].Name != "結束バンド" || meta.Artists[0].ID != 54103171 {
		t.Errorf("artist = %+v, want 結束バンド/54103171", meta.Artists[0])
	}
}

func TestArtistDecodesNameAndID(t *testing.T) {
	for _, raw := range []string{`["結束バンド", 54103171]`, `["結束バンド", "54103171"]`} {
		var artist Artist
		if err := json.Unmarshal([]byte(raw), &artist); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
		if artist.Name != "結束バンド" || artist.ID != 54103171 {
			t.Errorf("decode %s = %+v", raw, artist)
		}
	}
}

// TestArtistMalformedPayloadsDoNotPanic covers the shapes that used to panic on
// an unchecked type assertion or a missing index.
func TestArtistMalformedPayloadsDoNotPanic(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "empty array", raw: `[]`, wantErr: true},
		{name: "name only", raw: `["name"]`},
		{name: "extra elements", raw: `["name", 1, 2]`},
		{name: "object", raw: `{"name":"x"}`, wantErr: true},
		{name: "name is a number", raw: `[1, 2]`, wantErr: true},
		{name: "nested array as name", raw: `[["x"], 1]`, wantErr: true},
		{name: "null", raw: `null`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var artist Artist
			err := json.Unmarshal([]byte(tt.raw), &artist)
			if tt.wantErr && err == nil {
				t.Fatal("unmarshal succeeded, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
		})
	}
}

func TestMetaStringOmitsDerivedFields(t *testing.T) {
	meta := &Meta{
		ID:      7,
		Name:    "Track",
		Album:   &Album{ID: 9, Name: "Album"},
		Comment: "raw meta section",
	}

	got := meta.String()
	if !strings.Contains(got, `"musicName":"Track"`) {
		t.Errorf("String() = %s, want it to contain the track name", got)
	}
	if strings.Contains(got, "raw meta section") {
		t.Errorf("String() = %s, want it to omit the raw comment", got)
	}
}

func TestAlbumStringRendersJSON(t *testing.T) {
	album := &Album{ID: 9, Name: "Album", CoverURL: "https://example.invalid/cover.jpg"}

	got := album.String()
	if !strings.Contains(got, `"album":"Album"`) {
		t.Errorf("String() = %s, want it to contain the album name", got)
	}
}
