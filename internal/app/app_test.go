package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// mustWrite creates a file, and any directory leading to it.
func mustWrite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// TestFindNCMHonoursDepth pins the depth semantics: zero covers only the files
// directly inside a directory, and each further level descends one directory
// deeper.
func TestFindNCMHonoursDepth(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "top.ncm"))
	mustWrite(t, filepath.Join(root, "notes.txt"))
	mustWrite(t, filepath.Join(root, "one", "mid.ncm"))
	mustWrite(t, filepath.Join(root, "one", "two", "deep.ncm"))

	tests := []struct {
		name  string
		depth int
		want  []string
	}{
		{name: "zero covers the directory itself", depth: 0, want: []string{"top.ncm"}},
		{name: "one descends one level", depth: 1, want: []string{"one/mid.ncm", "top.ncm"}},
		{name: "two descends two levels", depth: 2, want: []string{"one/mid.ncm", "one/two/deep.ncm", "top.ncm"}},
		{name: "an excessive depth is harmless", depth: 99, want: []string{"one/mid.ncm", "one/two/deep.ncm", "top.ncm"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, err := findNCM(root, tt.depth)
			if err != nil {
				t.Fatalf("find ncm: %v", err)
			}

			got := make([]string, 0, len(found))
			for _, path := range found {
				relative, err := filepath.Rel(root, path)
				if err != nil {
					t.Fatalf("relative path: %v", err)
				}
				got = append(got, filepath.ToSlash(relative))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("findNCM(depth %d) = %v, want %v", tt.depth, got, tt.want)
			}
		})
	}
}

func TestFindNCMIgnoresUppercaseExtensionsCaseInsensitively(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "song.NCM"))

	found, err := findNCM(root, 0)
	if err != nil {
		t.Fatalf("find ncm: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("find ncm found %d files, want 1", len(found))
	}
}

func TestCollectSkipsDuplicates(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "song.ncm")
	mustWrite(t, source)

	got, err := collect([]string{source, root}, 0)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("collect returned %d paths, want 1: %v", len(got), got)
	}
}

func TestCollectReportsMissingInputs(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.ncm")
	if _, err := collect([]string{missing}, 0); err == nil {
		t.Fatal("collect succeeded on a missing input, want an error")
	}
}

func TestRunRejectsNonPositiveThreads(t *testing.T) {
	for _, threads := range []int{0, -1} {
		if err := Run(context.Background(), Options{Inputs: []string{"unused"}, Threads: threads}); err == nil {
			t.Errorf("Run with %d threads succeeded, want an error", threads)
		}
	}
}

func TestRunWithNoMatchesSucceeds(t *testing.T) {
	if err := Run(context.Background(), Options{Inputs: []string{t.TempDir()}, Threads: 1}); err != nil {
		t.Fatalf("run on a directory without containers: %v", err)
	}
}
