package cli

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncGraph(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		// text/ascii is the default: a colorized summary of units and edges.
		cliui.ChangeFormatterData(
			cliui.TextFormat(),
			func(data any) (any, error) {
				g, err := syncGraphFromData(data)
				if err != nil {
					return nil, err
				}
				return renderSyncGraphASCII(g), nil
			}),
		cliui.JSONFormat(),
		cliui.ChangeFormatterData(
			cliui.TableFormat([]syncGraphTableRow{}, []string{"unit", "status", "ready", "depends on"}),
			func(data any) (any, error) {
				g, err := syncGraphFromData(data)
				if err != nil {
					return nil, err
				}
				return syncGraphTableRows(g), nil
			}),
		dotFormat{},
	)

	cmd := &serpent.Command{
		Use:   "graph",
		Short: "Show the unit dependency graph",
		Long:  "Render the dependency graph of all units. Vertices are units and edges are declared dependencies. The default output is a colorized ASCII summary; use --output json for the raw graph, --output table for a flat listing, or --output dot for Graphviz DOT that can be piped to `dot`.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			events, err := client.SyncTimeline(ctx)
			if err != nil {
				// Older agents do not implement the SyncTimeline RPC.
				if strings.Contains(err.Error(), "unknown rpc") {
					return xerrors.New("agent does not support graph")
				}
				return xerrors.Errorf("get graph failed: %w", err)
			}

			graph := buildSyncGraph(unitEventsFromSocket(events))

			out, err := formatter.Format(ctx, graph)
			if err != nil {
				return xerrors.Errorf("format graph: %w", err)
			}

			_, _ = fmt.Fprintln(i.Stdout, out)

			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// syncGraphFromData recovers a syncGraph from the value passed through the
// output formatter.
func syncGraphFromData(data any) (syncGraph, error) {
	g, ok := data.(syncGraph)
	if !ok {
		return syncGraph{}, xerrors.Errorf("expected syncGraph, got %T", data)
	}
	return g, nil
}

// dotFormat is a custom output format that renders the graph in Graphviz
// DOT. It cannot reuse the text formatter via ChangeFormatterData because
// DataChangeFormat delegates ID() to the wrapped format, which would
// collide with the "text" format ID.
type dotFormat struct{}

var _ cliui.OutputFormat = dotFormat{}

func (dotFormat) ID() string { return "dot" }

func (dotFormat) AttachOptions(_ *serpent.OptionSet) {}

func (dotFormat) Format(_ context.Context, data any) (string, error) {
	g, err := syncGraphFromData(data)
	if err != nil {
		return "", err
	}
	return renderSyncGraphDOT(g), nil
}
