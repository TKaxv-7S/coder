package main

import (
	"errors"
	"os"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	releaserv1 "github.com/coder/coder/v2/scripts/releaser/v1"
	releaserv2 "github.com/coder/coder/v2/scripts/releaser/v2"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func main() {
	var (
		legacy bool
		dryRun bool
	)

	// The v2 tooling exposes exactly three subcommands: rc, branch, and
	// release. Each is run from a developer's command line and detects
	// the branch it is invoked from.
	children := releaserv2.Commands()

	// --legacy selects the v1 interactive wizard and cannot be combined
	// with a subcommand, which v1 does not understand.
	for _, c := range children {
		next := c.Handler
		c.Handler = func(inv *serpent.Invocation) error {
			if legacy {
				return xerrors.New("--legacy cannot be combined with a subcommand; run 'releaser --legacy' for the interactive tool")
			}
			return next(inv)
		}
	}

	cmd := &serpent.Command{
		Use:   "releaser <subcommand>",
		Short: "Release tooling for coder/coder.",
		Long: "Tag and publish releases for coder/coder.\n\n" +
			"By default releaser runs the non-interactive tooling via the rc,\n" +
			"branch, and release subcommands. Pass --legacy to run the older\n" +
			"interactive release wizard instead.",
		Options: serpent.OptionSet{
			{
				Name:        "legacy",
				Flag:        "legacy",
				Description: "Run the legacy interactive release wizard.",
				Value:       serpent.BoolOf(&legacy),
			},
			{
				Name:        "dry-run",
				Flag:        "dry-run",
				Description: "Print mutating commands instead of executing them (legacy wizard only).",
				Value:       serpent.BoolOf(&dryRun),
			},
		},
		Children: children,
		Handler: func(inv *serpent.Invocation) error {
			if legacy {
				return releaserv1.Run(inv, dryRun)
			}
			// No subcommand given and not in legacy mode: show help.
			return serpent.DefaultHelpFn()(inv)
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		if errors.Is(err, cliui.ErrCanceled) {
			os.Exit(1)
		}
		// Unwrap serpent's "running command ..." wrapper to keep output
		// clean.
		var runErr *serpent.RunCommandError
		if errors.As(err, &runErr) {
			err = runErr.Err
		}
		pretty.Fprintf(os.Stderr, cliui.DefaultStyles.Error, "Error: %s\n", err)
		os.Exit(1)
	}
}
