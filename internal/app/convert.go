package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/chilfish/ncmconverter/internal/converter"
	"github.com/chilfish/ncmconverter/internal/ncm"
	"github.com/chilfish/ncmconverter/internal/tag"
)

// convert decodes one container and writes the audio, and optionally the tags,
// to disk. It reports whether the destination was left alone because it
// already existed.
func convert(ctx context.Context, source string, opts Options) (skipped bool, err error) {
	file, err := ncm.Open(source)
	if err != nil {
		return false, err
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("closing ncm file failed", "file", source, "err", err)
		}
	}()

	if err := file.Parse(); err != nil {
		return false, err
	}

	decoded := converter.NewConverter(file)
	if err := decoded.HandleAll(); err != nil {
		return false, err
	}

	dest, err := outputPath(opts.Output, file, decoded.MetaData, decoded.MetaData.Format, opts.OutputTemplate)
	if err != nil {
		return false, err
	}
	if opts.SkipExisting {
		switch _, err := os.Stat(dest); {
		case err == nil:
			slog.Info("skipping a file that already exists", "source", source, "dest", dest)
			return true, nil
		case !errors.Is(err, fs.ErrNotExist):
			return false, fmt.Errorf("inspect %s: %w", dest, err)
		}
	}

	if err := writeFile(dest, decoded.MusicData); err != nil {
		return false, err
	}
	slog.Info("converted container", "source", source, "dest", dest)

	if !opts.Tag {
		return false, nil
	}

	// The lyrics live in a sidecar beside the container, because the format
	// itself has no room for them.
	lyrics, err := readLyrics(source)
	if err != nil {
		return false, err
	}
	if err := tag.WriteTo(ctx, dest, decoded.Cover.Bytes, decoded.MetaData, lyrics); err != nil {
		return false, fmt.Errorf("tag %s: %w", dest, err)
	}
	return false, nil
}

// outputPath places the converted file in output, or beside its source when
// output is empty.
//
// Without a template the source name is kept and only the extension is
// swapped. With one, the placeholders documented on Options.OutputTemplate are
// filled in, and the audio extension is appended. The result may not leave the
// output directory, however the metadata is spelled.
func outputPath(output string, file *ncm.File, meta *converter.Meta, format, tmpl string) (string, error) {
	dir := output
	if dir == "" {
		dir = file.FileDir
	}

	name := strings.TrimSuffix(file.FileName, file.Ext)
	if tmpl != "" {
		rendered := renderName(tmpl, file, meta, format)
		if rendered != "" {
			name = rendered
		}
	}

	dest := filepath.Join(dir, name+"."+format)
	rel, err := filepath.Rel(dir, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("output name %q escapes the output directory %s", name, dir)
	}
	return dest, nil
}

// renderName fills the placeholders a template may use. Anything the template
// does not name is passed through, so a name can contain literal braces.
//
// Substituted values have their path separators neutralised, so metadata can
// never turn a file name into a directory.
func renderName(tmpl string, file *ncm.File, meta *converter.Meta, format string) string {
	replacer := strings.NewReplacer(
		"{name}", sanitizeName(strings.TrimSuffix(file.FileName, file.Ext)),
		"{title}", sanitizeName(meta.Name),
		"{artist}", sanitizeName(firstArtistName(meta)),
		"{album}", sanitizeName(albumName(meta)),
		"{id}", sanitizeName(meta.ID.String()),
		"{format}", sanitizeName(format),
	)
	return strings.TrimSpace(replacer.Replace(tmpl))
}

// sanitizeName turns a value into something safe to drop into a file name:
// path separators become spaces and control characters are dropped.
func sanitizeName(value string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		default:
			return r
		}
	}, value)
	return strings.TrimSpace(cleaned)
}

// firstArtistName returns the name of the first credited artist, or an empty
// string when the metadata names none.
func firstArtistName(meta *converter.Meta) string {
	if meta == nil || len(meta.Artists) == 0 {
		return ""
	}
	return meta.Artists[0].Name
}

// albumName returns the album title, or an empty string when there is none.
func albumName(meta *converter.Meta) string {
	if meta == nil || meta.Album == nil {
		return ""
	}
	return meta.Album.Name
}

// writeFile writes data to dest, creating the parent directory when needed.
func writeFile(dest string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("write converted file: %w", err)
	}
	return nil
}
