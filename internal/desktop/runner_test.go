package desktop

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chilfish/ncmconverter/internal/ncmtest"
)

// frames returns deterministic audio bytes, easy to tell apart from tag data.
func frames(size int, seed byte) []byte {
	out := make([]byte, size)
	for i := range out {
		out[i] = byte(i) ^ seed
	}
	return out
}

// container writes a synthetic .ncm container into dir and returns its path.
func container(t *testing.T, dir, name string, seed byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	body := ncmtest.Build(ncmtest.Options{
		Meta:  map[string]any{"format": "mp3", "musicName": "Track"},
		Music: ncmtest.MinimalMP3(frames(256, seed)),
	})
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write container %s: %v", name, err)
	}
	return path
}

// namesIn returns the file names in dir with the given extension, sorted.
func namesIn(t *testing.T, dir, ext string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ext) {
			names = append(names, strings.TrimSuffix(entry.Name(), ext))
		}
	}
	slices.Sort(names)
	return names
}

// TestRunCountsEveryOutcome drives one converted, one skipped and one failed
// container through a single run and checks the tally.
func TestRunCountsEveryOutcome(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	container(t, dir, "good.ncm", 0x41)
	container(t, dir, "skip.ncm", 0x42)
	if err := os.WriteFile(filepath.Join(output, "skip.mp3"), []byte("already here"), 0o644); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.ncm"), []byte("not an ncm container at all"), 0o644); err != nil {
		t.Fatalf("write bad container: %v", err)
	}

	r := &runner{}
	result, err := r.convert(context.Background(), Request{
		Inputs:       []string{dir},
		Output:       output,
		Threads:      2,
		SkipExisting: true,
	}, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if result.Total != 3 || result.Converted != 1 || result.Skipped != 1 || result.Failed != 1 {
		t.Errorf("result = %+v, want total 3, converted 1, skipped 1, failed 1", result)
	}
	if _, err := os.Stat(filepath.Join(output, "good.mp3")); err != nil {
		t.Errorf("the converted file is missing: %v", err)
	}
}

// TestRunReportsFileFailuresWithoutAnError locks the promise that a per-file
// failure is visible in the reports rather than raised as a call error.
func TestRunReportsFileFailuresWithoutAnError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.ncm"), []byte("not an ncm container at all"), 0o644); err != nil {
		t.Fatalf("write bad container: %v", err)
	}

	var seen []Report
	r := &runner{}
	result, err := r.convert(context.Background(), Request{Inputs: []string{dir}, Threads: 1}, func(report Report) {
		seen = append(seen, report)
	})
	if err != nil {
		t.Fatalf("Run returned an error for a per-file failure: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("result = %+v, want one failure", result)
	}
	if len(seen) != 1 || seen[0].Err == "" {
		t.Errorf("reports = %+v, want one report carrying the reason", seen)
	}
}

// TestRunRejectsASecondRun locks single-flight: while a run is in flight,
// another is refused.
func TestRunRejectsASecondRun(t *testing.T) {
	r := &runner{}
	// Hold the guard the way a run in progress does, so the test does not
	// depend on timing a real run.
	if _, release, err := r.begin(context.Background()); err != nil {
		t.Fatalf("begin: %v", err)
	} else {
		defer r.finish(release)
	}

	if _, err := r.convert(context.Background(), Request{Threads: 1}, nil); err == nil {
		t.Fatal("Run accepted a second run, want an error")
	}
}

// TestRunNormalizesCancellation locks the promise that a cancelled run reports
// itself as cancelled rather than failing.
func TestRunNormalizesCancellation(t *testing.T) {
	dir := t.TempDir()
	container(t, dir, "song.ncm", 0x43)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &runner{}
	result, err := r.convert(ctx, Request{Inputs: []string{dir}, Threads: 1}, nil)
	if err != nil {
		t.Fatalf("Run returned an error for a cancellation: %v", err)
	}
	if !result.Canceled {
		t.Errorf("result = %+v, want Canceled", result)
	}
}

// TestScanMatchesWhatARunHandles locks Scan and Run to the same traversal: what
// Scan lists is what Run writes, down to the depth boundary.
func TestScanMatchesWhatARunHandles(t *testing.T) {
	dir := t.TempDir()
	container(t, dir, "top.ncm", 0x44)

	nested := filepath.Join(dir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create subdirectory: %v", err)
	}
	container(t, nested, "nested.ncm", 0x45)

	// Beyond the depth under test, so it must be left out of both.
	deep := filepath.Join(nested, "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	container(t, deep, "deep.ncm", 0x46)

	scanned, err := scan([]string{dir}, 1)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	output := t.TempDir()
	r := &runner{}
	if _, err := r.convert(context.Background(), Request{
		Inputs:  []string{dir},
		Output:  output,
		Depth:   1,
		Threads: 1,
	}, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	handled := namesIn(t, output, ".mp3")
	want := make([]string, 0, len(scanned))
	for _, path := range scanned {
		want = append(want, strings.TrimSuffix(filepath.Base(path), ".ncm"))
	}
	slices.Sort(want)
	if !slices.Equal(handled, want) {
		t.Errorf("Scan listed %v but Run handled %v", want, handled)
	}
}

// TestRunRejectsNonPositiveThreads locks the promise that the thread count is
// validated, as internal/app does.
func TestRunRejectsNonPositiveThreads(t *testing.T) {
	r := &runner{}
	if _, err := r.convert(context.Background(), Request{Inputs: []string{"unused"}, Threads: 0}, nil); err == nil {
		t.Fatal("Run with 0 threads succeeded, want an error")
	}
}
