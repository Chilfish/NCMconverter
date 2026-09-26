// Command ncmconverter converts NetEase Cloud Music (.ncm) files into mp3 or
// flac files.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/chilfish/ncmconverter/internal/app"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	// Report the commit and build date alongside the version, so a binary
	// someone is running can be traced back to a revision.
	cli.VersionPrinter = func(cmd *cli.Command) {
		_, _ = fmt.Fprintf(cmd.Root().Writer, "%s %s (commit %s, built %s)\n",
			cmd.Name, cmd.Version, commit, date)
	}

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
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.NArg() == 0 {
				return errors.New("no input files or directories given, run with --help for usage")
			}
			return app.Run(ctx, app.Options{
				Inputs:  cmd.Args().Slice(),
				Output:  cmd.String("output"),
				Tag:     cmd.Bool("tag"),
				Depth:   cmd.Int("depth"),
				Threads: cmd.Int("threads"),
			})
		},
	}

	if err := command.Run(ctx, os.Args); err != nil {
		slog.Error("ncmconverter failed", "err", err)
		os.Exit(1)
	}
}
