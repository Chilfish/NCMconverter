package app

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chilfish/ncmconverter/internal/converter"
	"github.com/chilfish/ncmconverter/internal/ncm"
	"github.com/chilfish/ncmconverter/internal/ncmtest"
)

// mp3Container writes a container that converts to an mp3 of the given name.
func mp3Container(t *testing.T, dir, name string, meta map[string]any, seed byte) string {
	t.Helper()
	if meta == nil {
		meta = map[string]any{"format": "mp3"}
	}
	meta["format"] = "mp3"
	return container(t, dir, name, ncmtest.Options{
		Meta:  meta,
		Music: ncmtest.MinimalMP3(frames(256, seed)),
	})
}

// TestRunDryRunWritesNothing checks that a dry run reports the work without
// touching the disk at all.
func TestRunDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "converted")
	source := mp3Container(t, dir, "song.ncm", nil, 0x77)

	if err := Run(context.Background(), Options{
		Inputs:  []string{source},
		Output:  output,
		Tag:     true,
		Threads: 1,
		DryRun:  true,
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read input directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "song.ncm" {
		t.Errorf("dry run left %d entries in the input directory, want only the container", len(entries))
	}
	if _, err := os.Stat(output); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry run created the output directory")
	}
}

// TestRunSkipExistingLeavesTheDestinationAlone locks the promise that
// --skip-existing does not touch a converted file that is already there.
func TestRunSkipExistingLeavesTheDestinationAlone(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x78)

	sentinel := []byte("leave me alone")
	dest := filepath.Join(output, "song.mp3")
	if err := os.WriteFile(dest, sentinel, 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	if err := Run(context.Background(), Options{
		Inputs:       []string{source},
		Output:       output,
		Tag:          true,
		Threads:      1,
		SkipExisting: true,
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if got := readFile(t, dest); !bytes.Equal(got, sentinel) {
		t.Error("the existing file was overwritten")
	}
}

// TestRunOverwritesByDefault makes sure --skip-existing is what changes the
// behaviour, not the other way round.
func TestRunOverwritesByDefault(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x79)

	dest := filepath.Join(output, "song.mp3")
	if err := os.WriteFile(dest, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	if err := Run(context.Background(), Options{
		Inputs:  []string{source},
		Output:  output,
		Tag:     false,
		Threads: 1,
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if got := readFile(t, dest); bytes.Equal(got, []byte("stale")) {
		t.Error("the existing file was not overwritten")
	}
}

func TestRunOutputTemplateNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", map[string]any{
		"musicName": "Track",
		"artist":    [][]any{{"結束バンド", 1}},
		"album":     "ドッペルゲンガー",
	}, 0x7a)

	if err := Run(context.Background(), Options{
		Inputs:         []string{source},
		Output:         output,
		Threads:        1,
		OutputTemplate: "{artist} - {album}",
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	want := filepath.Join(output, "結束バンド - ドッペルゲンガー.mp3")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected the templated name: %v", err)
	}
}

// TestRunOutputTemplateFallsBackWhenItRendersEmpty covers a template whose
// fields the metadata does not have.
func TestRunOutputTemplateFallsBackWhenItRendersEmpty(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x7b)

	if err := Run(context.Background(), Options{
		Inputs:         []string{source},
		Output:         output,
		Threads:        1,
		OutputTemplate: "{artist}",
	}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, "song.mp3")); err != nil {
		t.Fatalf("expected the source name to be kept: %v", err)
	}
}

// TestRunOutputTemplateCannotEscapeTheOutputDirectory locks the path check: a
// template that walks up has to fail rather than write outside the directory.
func TestRunOutputTemplateCannotEscapeTheOutputDirectory(t *testing.T) {
	dir := t.TempDir()
	output := t.TempDir()
	source := mp3Container(t, dir, "song.ncm", nil, 0x7c)

	err := Run(context.Background(), Options{
		Inputs:         []string{source},
		Output:         output,
		Threads:        1,
		OutputTemplate: "../{name}",
	})
	if err == nil {
		t.Fatal("run succeeded with a template that escapes the output directory, want an error")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(output), "song.mp3")); statErr == nil {
		t.Error("the escaping template wrote outside the output directory")
	}
}

// TestRenderNameNeutralisesSeparatorsInMetadata checks that a value from the
// container cannot introduce a directory of its own.
func TestRenderNameNeutralisesSeparatorsInMetadata(t *testing.T) {
	got := sanitizeName(`a/b\c`)
	if strings.ContainsAny(got, `/\`) {
		t.Errorf("sanitizeName(%q) = %q, still holds a path separator", `a/b\c`, got)
	}

	meta := &converter.Meta{Name: "../../escape"}
	if name := renderName("{title}", &ncm.File{FileName: "song.ncm", Ext: ".ncm"}, meta, "mp3"); strings.ContainsAny(name, `/\`) {
		t.Errorf("renderName kept a path separator: %q", name)
	}
}

// TestRenderNameFillsEveryDocumentedPlaceholder keeps the flag's help text and
// the implementation from drifting apart.
func TestRenderNameFillsEveryDocumentedPlaceholder(t *testing.T) {
	file := &ncm.File{FileName: "song.ncm", Ext: ".ncm"}
	meta := &converter.Meta{
		ID:      42,
		Name:    "Track",
		Artists: []converter.Artist{{Name: "Artist"}},
		Album:   &converter.Album{Name: "Album"},
	}

	const tmpl = "{name}|{title}|{artist}|{album}|{id}|{format}"
	want := "song|Track|Artist|Album|42|mp3"
	if got := renderName(tmpl, file, meta, "mp3"); got != want {
		t.Errorf("renderName = %q, want %q", got, want)
	}
}
