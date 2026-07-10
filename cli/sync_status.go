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

func (*RootCmd) syncStatus(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat(
				[]agentsocket.DependencyInfo{},
				[]string{
					"depends on",
					"required status",
					"current status",
					"satisfied",
				},
			),
			func(data any) (any, error) {
				resp, ok := data.(agentsocket.SyncStatusResponse)
				if !ok {
					return nil, xerrors.Errorf("expected agentsocket.SyncStatusResponse, got %T", data)
				}
				return resp.Dependencies, nil
			}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "status <unit>",
		Short: "Show unit status and dependency state",
		Long:  "Show the current status of a unit, whether it is ready to start, and lists its dependencies. Shows which dependencies are satisfied and which are still pending.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unit := unit.ID(i.Args[0])

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			statusResp, err := client.SyncStatus(ctx, unit)
			if err != nil {
				return xerrors.Errorf("get status failed: %w", err)
			}

			var out string
			if formatter.FormatID() == "table" {
				header := fmt.Sprintf("Unit: %s\nStatus: %s\nReady: %t\n\nDependencies:\n", unit, statusResp.Status, statusResp.IsReady)
				dependencies := "No dependencies found"
				if len(statusResp.Dependencies) > 0 {
					dependencies, err = formatter.Format(ctx, statusResp)
					if err != nil {
						return xerrors.Errorf("format status: %w", err)
					}
					dependencies = strings.TrimRight(dependencies, "\n")
				}
				history := "No history found"
				if len(statusResp.History) > 0 {
					history, err = cliui.DisplayTable(syncHistoryRows(statusResp.History), "", nil)
					if err != nil {
						return xerrors.Errorf("format history: %w", err)
					}
					history = strings.TrimRight(history, "\n")
				}
				out = header + dependencies + "\n\nHistory:\n" + history
			} else {
				out, err = formatter.Format(ctx, statusResp)
				if err != nil {
					return xerrors.Errorf("format status: %w", err)
				}
			}

			_, _ = fmt.Fprintln(i.Stdout, out)

			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

// syncHistoryRow is one rendered line of a unit's event history.
type syncHistoryRow struct {
	Time    string `table:"time,nosort"`
	Elapsed string `table:"elapsed"`
	Event   string `table:"event"`
}

// syncHistoryRows renders unit events as history table rows. Times are
// shown in UTC; elapsed offsets are relative to the first event.
func syncHistoryRows(history []agentsocket.UnitEvent) []syncHistoryRow {
	rows := make([]syncHistoryRow, 0, len(history))
	base := history[0].Time
	for _, ev := range unitEventsFromSocket(history) {
		rows = append(rows, syncHistoryRow{
			Time:    ev.Time.UTC().Format("15:04:05.000"),
			Elapsed: syncElapsed(base, ev.Time),
			Event:   syncEventDescription(ev),
		})
	}
	return rows
}
