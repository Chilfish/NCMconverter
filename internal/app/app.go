// Package app orchestrates the conversion of NCM containers into audio files.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Options controls a conversion run.
type Options struct {
	// Inputs are the files and directories to convert.
	Inputs []string
	// Output is the destination directory. An empty value writes every result
	// next to the container it came from.
	Output string
	// OutputTemplate names the converted file. An empty value keeps the source
	// name. Available placeholders are {name} (the source name without its
	// extension), {title}, {artist}, {album}, {id} and {format}; the audio
	// extension is always appended. The result may not leave the output
	// directory.
	OutputTemplate string
	// Tag controls whether metadata and cover art are written into the result.
	Tag bool
	// Depth is how many levels below each input directory are searched. Zero
	// converts only the immediate contents of a directory.
	Depth int
	// Threads is the maximum number of files converted concurrently.
	Threads int
	// DryRun reports what would be converted without writing anything.
	DryRun bool
	// SkipExisting leaves a destination that already exists untouched instead
	// of overwriting it. The file counts as handled, not as a failure.
	SkipExisting bool
}

// Run converts every .ncm file reachable from the configured inputs.
//
// A file that fails to convert is reported and skipped, so that one bad
// container does not abandon a whole batch. The returned error is non-nil when
// any file failed, or when the context is cancelled.
func Run(ctx context.Context, opts Options) error {
	if opts.Threads < 1 {
		return fmt.Errorf("threads must be at least 1, got %d", opts.Threads)
	}

	sources, err := collect(opts.Inputs, opts.Depth)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		slog.Warn("no .ncm files to convert")
		return nil
	}
	if opts.DryRun {
		for _, source := range sources {
			slog.Info("would convert", "file", source)
		}
		return nil
	}

	var (
		mu       sync.Mutex
		failures []error
		skipped  int
	)
	group := new(errgroup.Group)
	group.SetLimit(opts.Threads)
	for _, source := range sources {
		group.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			wasSkipped, err := convert(ctx, source, opts)
			if err != nil {
				slog.Error("conversion failed", "file", source, "err", err)
				mu.Lock()
				failures = append(failures, fmt.Errorf("%s: %w", source, err))
				mu.Unlock()
				return nil
			}
			if wasSkipped {
				mu.Lock()
				skipped++
				mu.Unlock()
			}
			return nil
		})
	}
	// Cancellation is the only failure that reaches Wait; per-file failures are
	// collected above so that the rest of the batch still runs.
	if err := group.Wait(); err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d of %d files failed: %w", len(failures), len(sources), errors.Join(failures...))
	}

	slog.Info("converted files", "count", len(sources)-skipped, "skipped", skipped)
	return nil
}

// collect expands the given paths into the .ncm files they hold, skipping
// duplicates when inputs overlap.
func collect(inputs []string, depth int) ([]string, error) {
	seen := make(map[string]struct{})
	sources := make([]string, 0, len(inputs))

	add := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		sources = append(sources, path)
	}

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return nil, fmt.Errorf("inspect input: %w", err)
		}
		if !info.IsDir() {
			add(filepath.Clean(input))
			continue
		}
		found, err := findNCM(input, depth)
		if err != nil {
			return nil, err
		}
		for _, path := range found {
			add(path)
		}
	}
	return sources, nil
}

// findNCM returns every .ncm file within dir, descending depth levels into
// subdirectories. A depth of zero covers only the files directly inside dir.
func findNCM(dir string, depth int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	found := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if depth <= 0 {
				continue
			}
			nested, err := findNCM(path, depth-1)
			if err != nil {
				return nil, err
			}
			found = append(found, nested...)
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".ncm") {
			found = append(found, path)
		}
	}
	return found, nil
}
