package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncTimeline(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TextFormat(),
			func(data any) (any, error) {
				events, ok := data.([]agentsocket.UnitEvent)
				if !ok {
					return nil, xerrors.Errorf("expected []agentsocket.UnitEvent, got %T", data)
				}
				return renderTimeline(unitEventsFromSocket(events)), nil
			}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "timeline",
		Short: "Show a timeline of unit state changes",
		Long:  "Show every recorded unit state transition and dependency declaration across all units, in the order they occurred. The default output is an ASCII graph of the dependency DAG in event-time order; use --output json for the raw event list.",
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
					return xerrors.New("agent does not support timeline")
				}
				return xerrors.Errorf("get timeline failed: %w", err)
			}

			out, err := formatter.Format(ctx, events)
			if err != nil {
				return xerrors.Errorf("format timeline: %w", err)
			}

			_, _ = fmt.Fprintln(i.Stdout, out)

			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// unitEventsFromSocket converts socket client event wrappers back to
// unit.Event values for rendering and derivation.
func unitEventsFromSocket(events []agentsocket.UnitEvent) []unit.Event {
	out := make([]unit.Event, 0, len(events))
	for _, ev := range events {
		out = append(out, unit.Event{
			Seq:            ev.Seq,
			Time:           ev.Time,
			Kind:           ev.Kind,
			Unit:           ev.Unit,
			From:           ev.From,
			To:             ev.To,
			DependsOn:      ev.DependsOn,
			RequiredStatus: ev.RequiredStatus,
		})
	}
	return out
}
