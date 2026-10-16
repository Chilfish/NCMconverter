package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// collectResults returns an OnFile callback that appends every result it is
// given. Run calls it under its own lock, so the append needs no synchronization.
func collectResults(got *[]FileResult) func(FileResult) {
	return func(result FileResult) {
		*got = append(*got, result)
	}
}

// TestRunReportsPlannedFilesOnADryRun locks the promise that a dry run calls
// back once per file with OutcomePlanned, and still writes nothing.
func TestRunReportsPlannedFilesOnADryRun(t *testing.T) {
	dir := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x31)

	var got []FileResult
	if err := Run(context.Background(), Options{
		Inputs:  []string{source},
		Threads: 1,
		DryRun:  true,
		OnFile:  collectResults(&got),
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("reported %d files, want 1: %+v", len(got), got)
	}
	if got[0].Source != source || got[0].Outcome != OutcomePlanned {
		t.Errorf("reported %+v, want the source and OutcomePlanned", got[0])
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read input directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "song.ncm" {
		t.Errorf("dry run left %d entries in the input directory, want only the container", len(entries))
	}
}

// TestRunReportsConvertedFiles locks the promise that a converted file is
// reported with the destination that was actually written.
func TestRunReportsConvertedFiles(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x32)

	var got []FileResult
	if err := Run(context.Background(), Options{
		Inputs:  []string{source},
		Output:  output,
		Threads: 1,
		OnFile:  collectResults(&got),
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("reported %d files, want 1: %+v", len(got), got)
	}
	want := filepath.Join(output, "song.mp3")
	if got[0].Outcome != OutcomeConverted || got[0].Dest != want || got[0].Err != nil {
		t.Errorf("reported %+v, want OutcomeConverted with dest %s and no error", got[0], want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("the reported destination is not on disk: %v", err)
	}
}

// TestRunReportsSkippedFiles locks the promise that --skip-existing reports the
// destination it left alone, without rewriting it.
func TestRunReportsSkippedFiles(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x33)

	dest := filepath.Join(output, "song.mp3")
	sentinel := []byte("leave me alone")
	if err := os.WriteFile(dest, sentinel, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	var got []FileResult
	if err := Run(context.Background(), Options{
		Inputs:       []string{source},
		Output:       output,
		Threads:      1,
		SkipExisting: true,
		OnFile:       collectResults(&got),
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(got) != 1 || got[0].Outcome != OutcomeSkipped {
		t.Fatalf("reported %+v, want one OutcomeSkipped", got)
	}
	if got[0].Dest != dest {
		t.Errorf("reported dest %q, want %q", got[0].Dest, dest)
	}
	if !bytes.Equal(readFile(t, dest), sentinel) {
		t.Error("the skipped destination was overwritten")
	}
}

// TestRunReportsFailures locks the promise that a bad container is reported
// with its error while the rest of the batch is still converted.
func TestRunReportsFailures(t *testing.T) {
	dir := t.TempDir()
	good := mp3Container(t, dir, "good.ncm", nil, 0x34)
	bad := filepath.Join(dir, "bad.ncm")
	if err := os.WriteFile(bad, []byte("not an ncm container at all"), 0o644); err != nil {
		t.Fatalf("write bad container: %v", err)
	}

	var got []FileResult
	err := Run(context.Background(), Options{
		Inputs:  []string{dir},
		Threads: 2,
		OnFile:  collectResults(&got),
	})
	if err == nil {
		t.Fatal("run succeeded with an unusable container, want an error")
	}

	if len(got) != 2 {
		t.Fatalf("reported %d files, want 2: %+v", len(got), got)
	}
	bySource := make(map[string]FileResult, len(got))
	for _, result := range got {
		bySource[result.Source] = result
	}
	if failed := bySource[bad]; failed.Outcome != OutcomeFailed || failed.Err == nil {
		t.Errorf("bad container reported %+v, want OutcomeFailed with an error", failed)
	}
	if converted := bySource[good]; converted.Outcome != OutcomeConverted {
		t.Errorf("good container reported %+v, want OutcomeConverted", converted)
	}
}

// TestRunWithoutACallbackStillSucceeds keeps the callback purely additive: a
// run without one behaves exactly as it did before the callback existed.
func TestRunWithoutACallbackStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x35)

	if err := Run(context.Background(), Options{Inputs: []string{source}, Threads: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "song.mp3")); err != nil {
		t.Errorf("the container was not converted: %v", err)
	}
}
