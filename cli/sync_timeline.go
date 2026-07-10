package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncTimeline(socketPath *string) *serpent.Command {
	var watch bool

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
		Long:  "Show every recorded unit state transition and dependency declaration across all units, in the order they occurred. The default output is an ASCII graph of the dependency DAG in event-time order; use --output json for the raw event list. With --watch, new rows are streamed as events occur until interrupted.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if watch && formatter.FormatID() != "text" {
				return xerrors.New("--watch only supports text output")
			}

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

			if watch {
				return watchTimeline(ctx, i, client, events)
			}

			out, err := formatter.Format(ctx, events)
			if err != nil {
				return xerrors.Errorf("format timeline: %w", err)
			}

			_, _ = fmt.Fprintln(i.Stdout, out)

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "watch",
			FlagShorthand: "w",
			Description:   "Stream new rows as events occur until interrupted.",
			Value:         serpent.BoolOf(&watch),
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// syncWatchPollInterval is how often --watch polls the agent for new
// events.
const syncWatchPollInterval = time.Second

// watchTimeline renders the events seen so far, then polls the agent and
// streams newly produced graph rows until the context is canceled.
func watchTimeline(ctx context.Context, i *serpent.Invocation, client *agentsocket.Client, events []agentsocket.UnitEvent) error {
	renderer := newTimelineRenderer()
	_, _ = fmt.Fprint(i.Stdout, renderer.renderEvents(unitEventsFromSocket(events)))

	ticker := time.NewTicker(syncWatchPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Interrupting a watch is the normal way to end it.
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
			events, err := client.SyncTimeline(ctx)
			if err != nil {
				if errors.Is(ctx.Err(), context.Canceled) {
					return nil
				}
				return xerrors.Errorf("get timeline failed: %w", err)
			}
			_, _ = fmt.Fprint(i.Stdout, renderer.renderEvents(unitEventsFromSocket(events)))
		}
	}
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
