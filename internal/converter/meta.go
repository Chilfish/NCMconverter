package converter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Int64 is an integer that NCM metadata may encode either as a JSON number or
// as a JSON string. Decoding into it never panics, and it never loses the
// precision of the large track and album identifiers.
type Int64 int64

// UnmarshalJSON accepts a JSON number, a numeric string, or null.
func (n *Int64) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(data), `"`)
	if text == "" || text == "null" {
		*n = 0
		return nil
	}
	if value, err := strconv.ParseInt(text, 10, 64); err == nil {
		*n = Int64(value)
		return nil
	}
	// A few tracks encode these fields as floating point numbers.
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("decode integer %q: %w", text, err)
	}
	*n = Int64(value)
	return nil
}

// String returns the value formatted as a decimal integer.
func (n Int64) String() string { return strconv.FormatInt(int64(n), 10) }

// Meta describes an audio track.
type Meta struct {
	ID       Int64    `json:"musicId"`
	Name     string   `json:"musicName"`
	Artists  []Artist `json:"artist"`
	BitRate  Int64    `json:"bitrate"`
	Duration Int64    `json:"duration"`
	Format   string   `json:"format"`

	// Album is populated from the same payload as the fields above.
	Album *Album `json:"-"`
	// Comment holds the raw meta section, which identifies the source file.
	Comment string `json:"-"`
}

// Album describes the release a track belongs to.
type Album struct {
	ID       Int64  `json:"albumId"`
	Name     string `json:"album"`
	CoverURL string `json:"albumPic"`
}

// Artist is a performer credited on a track. NCM encodes artists as two element
// arrays holding a name and an identifier.
type Artist struct {
	Name string
	ID   Int64
}

// UnmarshalJSON decodes the [name, id] array form used by NCM metadata.
func (a *Artist) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode artist: %w", err)
	}
	if len(fields) == 0 {
		return fmt.Errorf("decode artist: empty array")
	}
	if err := json.Unmarshal(fields[0], &a.Name); err != nil {
		return fmt.Errorf("decode artist name: %w", err)
	}
	if len(fields) > 1 {
		if err := json.Unmarshal(fields[1], &a.ID); err != nil {
			return fmt.Errorf("decode artist id: %w", err)
		}
	}
	return nil
}

// String renders the track metadata as JSON, for logging.
func (m *Meta) String() string {
	encoded, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf("meta{%s}", err)
	}
	return string(encoded)
}

// String renders the album metadata as JSON, for logging.
func (a *Album) String() string {
	encoded, err := json.Marshal(a)
	if err != nil {
		return fmt.Sprintf("album{%s}", err)
	}
	return string(encoded)
}
