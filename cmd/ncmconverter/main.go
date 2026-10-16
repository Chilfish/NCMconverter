// Command ncmconverter converts NetEase Cloud Music (.ncm) files into mp3 or
// flac files.
//
// Started without arguments it opens a desktop window; started with arguments
// it is the command line tool, unchanged. Both share internal/app, so the
// conversion itself has one implementation.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/chilfish/ncmconverter/internal/app"
	"github.com/chilfish/ncmconverter/internal/desktop"
	"github.com/urfave/cli/v3"
)

// Build information, overridden at build time. For example:
//
//	-ldflags "-X main.version=v1.2.3 -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%FT%TZ)"
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Arguments mean the command line tool; no arguments mean the desktop app.
	// mygo generate builds and runs this binary with MYGO_GENERATE set to write
	// the TypeScript client and return, which must take the desktop branch
	// rather than be mistaken for a command line call.
	if len(os.Args) > 1 && os.Getenv("MYGO_GENERATE") == "" {
		// A windowsgui build has no console of its own, so the shim has to
		// attach one before the logger captures os.Stderr.
		attachConsole()
		runCLI()
		return
	}
	if err := desktop.Run(); err != nil {
		slog.Error("ncmconverter failed", "err", err)
		os.Exit(1)
	}
}

// runCLI is the command line tool: the behaviour, flags, defaults, log output
// and exit codes are exactly what the binary did before the desktop app
// existed.
func runCLI() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Report the commit and build date alongside the version, so a binary
	// someone is running can be traced back to a revision.
	cli.VersionPrinter = func(cmd *cli.Command) {
		_, _ = fmt.Fprintf(cmd.Root().Writer, "%s %s (commit %s, built %s)\n",
			cmd.Name, cmd.Version, commit, date)
	}

	// The level is raised from --quiet once the flags are parsed, which happens
	// inside Run.
	logLevel := new(slog.LevelVar)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))

	command := &cli.Command{
		Name:      "ncmconverter",
		Usage:     "convert ncm files to mp3 or flac",
		ArgsUsage: "<files/dirs>",
		Version:   version,
		Authors:   []any{"Chilfish <chill4fish@gmail.com>"},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "write converted files into this directory instead of beside their source",
			},
			&cli.StringFlag{
				Name:  "output-template",
				Usage: "name converted files from {name}, {title}, {artist}, {album}, {id} and {format}",
			},
			&cli.BoolFlag{
				Name:    "tag",
				Aliases: []string{"t"},
				Value:   true,
				Usage:   "write metadata and cover art into the converted file; pass --tag=false to skip it",
			},
			&cli.IntFlag{
				Name:    "depth",
				Aliases: []string{"d", "deepth"},
				Value:   0,
				Usage:   "how many directory levels below each input directory to search",
			},
			&cli.IntFlag{
				Name:    "threads",
				Aliases: []string{"n", "thread"},
				Value:   10,
				Usage:   "maximum number of files to convert concurrently",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "list the files that would be converted and write nothing",
			},
			&cli.BoolFlag{
				Name:  "skip-existing",
				Usage: "leave a converted file that already exists untouched instead of overwriting it",
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "log only warnings and errors",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() == 0 {
				return errors.New("no input files or directories given, run with --help for usage")
			}
			if cmd.Bool("quiet") {
				logLevel.Set(slog.LevelWarn)
			}
			return app.Run(ctx, app.Options{
				Inputs:         cmd.Args().Slice(),
				Output:         cmd.String("output"),
				OutputTemplate: cmd.String("output-template"),
				Tag:            cmd.Bool("tag"),
				Depth:          cmd.Int("depth"),
				Threads:        cmd.Int("threads"),
				DryRun:         cmd.Bool("dry-run"),
				SkipExisting:   cmd.Bool("skip-existing"),
			})
		},
	}

	if err := command.Run(ctx, os.Args); err != nil {
		slog.Error("ncmconverter failed", "err", err)
		os.Exit(1)
	}
}
