package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/chilfish/ncmconverter/internal/converter"
	"github.com/chilfish/ncmconverter/internal/ncm"
	"github.com/chilfish/ncmconverter/internal/tag"
)

// convert decodes one container and writes the audio, and optionally the tags,
// to disk.
func convert(ctx context.Context, source string, opts Options) error {
	file, err := ncm.Open(source)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("closing ncm file failed", "file", source, "err", err)
		}
	}()

	if err := file.Parse(); err != nil {
		return err
	}

	decoded := converter.NewConverter(file)
	if err := decoded.HandleAll(); err != nil {
		return err
	}

	dest := outputPath(opts.Output, file, decoded.MetaData.Format)
	if err := writeFile(dest, decoded.MusicData); err != nil {
		return err
	}
	slog.Info("converted container", "source", source, "dest", dest)

	if !opts.Tag {
		return nil
	}

	// The lyrics live in a sidecar beside the container, because the format
	// itself has no room for them.
	lyrics, err := readLyrics(source)
	if err != nil {
		return err
	}
	if err := tag.WriteTo(ctx, dest, decoded.Cover.Bytes, decoded.MetaData, lyrics); err != nil {
		return fmt.Errorf("tag %s: %w", dest, err)
	}
	return nil
}

// outputPath places the converted file in output, or beside its source when
// output is empty, keeping the source name and swapping the extension.
func outputPath(output string, file *ncm.File, format string) string {
	dir := output
	if dir == "" {
		dir = file.FileDir
	}
	name := strings.TrimSuffix(file.FileName, file.Ext) + "." + format
	return filepath.Join(dir, name)
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
