package chatd

import (
	"context"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/apiversion"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
)

const (
	// contextReportPollInterval is how often the context-report gate
	// re-resolves the chat's agent and re-checks for a pushed snapshot
	// while a turn waits for the agent's first context report.
	contextReportPollInterval = time.Second
	// defaultContextReportTimeout bounds how long a turn waits for the
	// workspace agent to report its context snapshot before failing the
	// turn with a visible error.
	defaultContextReportTimeout = 15 * time.Minute

	contextReportTickerTag     = "context-report-tick"
	contextReportTimeoutTag    = "context-report-timeout"
	contextReportTimerTagGroup = "chatd"

	// Context pushes require Agent API >= 2.10 (PushContextState). Agents
	// built before that version can never report context, so waiting on
	// them is pointless.
	contextReportMinAPIMajor = 2
	contextReportMinAPIMinor = 10
)

var (
	errChatContextWorkspaceNotStarted = newChatContextGateError(
		"workspace must be started to report chat context")
	errChatContextAgentTooOld = newChatContextGateError(
		"workspace agent is too old to report chat context; " +
			"update Coder and rebuild the workspace")
)

// newChatContextGateError builds a context-gate error whose text survives
// chaterror classification, so the exact message lands in the chat's
// persisted last_error instead of a generic fallback.
func newChatContextGateError(message string) error {
	return chaterror.WithClassification(
		xerrors.New(message),
		chaterror.ClassifiedError{
			Message: message,
			Kind:    codersdk.ChatErrorKindGeneric,
		},
	)
}

// awaitChatContextReported enforces that a workspace-bound turn does not run
// before the chat's agent has reported a context snapshot. A chat whose
// current snapshot is already pinned proceeds immediately; agent resolution
// errors are swallowed on that path so pinned chats on stopped (zero-agent)
// workspaces keep working. An unpinned chat waits on a poll loop that
// re-resolves the agent (rebinding to a newer start build when the binding is
// stale) and exits as soon as the bound agent has any snapshot row, then pins
// the chat to it.
//
// The wait fails fast, without burning the ceiling, when the workspace's
// latest build is not a start transition or when the bound agent connected
// with an Agent API too old to push context. Context cancellation propagates
// unchanged so interrupts keep working. Terminal failures are wrapped with
// terminalGeneration so the prepare phase does not retry them and the message
// surfaces as the chat's visible error state.
func (server *Server) awaitChatContextReported(
	ctx context.Context,
	workspaceCtx *turnWorkspaceContext,
	logger slog.Logger,
) (database.WorkspaceAgent, error) {
	before := workspaceCtx.currentChatSnapshot()
	resolvedChat, agent, resolveErr := workspaceCtx.ensureWorkspaceAgent(ctx)
	if resolveErr == nil &&
		(!nullUUIDEqual(before.AgentID, resolvedChat.AgentID) ||
			!nullUUIDEqual(before.BuildID, resolvedChat.BuildID)) {
		// The resolution rebound the chat to another build's agent and
		// re-pinned its context in a separate transaction, so the cached
		// snapshot's pinned fields are stale. Reload before reading them.
		fresh, err := workspaceCtx.loadChatSnapshot(ctx, resolvedChat.ID)
		if err != nil {
			return database.WorkspaceAgent{}, xerrors.Errorf(
				"reload chat after agent rebind: %w", err)
		}
		workspaceCtx.setCurrentChat(fresh)
	}

	if chatContextPinned(workspaceCtx.currentChatSnapshot()) {
		// Pinned and, because ensureWorkspaceAgent rebinds stale bindings
		// and rebinding re-pins, current. Proceed as before the gate
		// existed: resolveErr is swallowed so a pinned chat on a stopped
		// workspace with no agents still runs from its pinned context.
		if resolveErr != nil {
			logger.Debug(ctx, "context gate: proceeding with pinned context despite agent resolution error",
				slog.Error(resolveErr))
		}
		return agent, nil
	}
	if ctx.Err() != nil {
		return database.WorkspaceAgent{}, ctx.Err()
	}

	agent, done, err := server.checkChatContextReported(ctx, workspaceCtx, logger)
	if err != nil {
		return database.WorkspaceAgent{}, err
	}
	if done {
		return agent, nil
	}

	logger.Info(ctx, "context gate: waiting for workspace agent to report chat context",
		slog.F("timeout", server.contextReportTimeout))

	timeout := server.clock.NewTimer(server.contextReportTimeout,
		contextReportTimerTagGroup, contextReportTimeoutTag)
	defer timeout.Stop()
	ticker := server.clock.NewTicker(contextReportPollInterval,
		contextReportTimerTagGroup, contextReportTickerTag)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return database.WorkspaceAgent{}, ctx.Err()
		case <-timeout.C:
			return database.WorkspaceAgent{}, terminalGeneration(newChatContextGateError(
				"workspace agent did not report chat context within " +
					server.contextReportTimeout.String()))
		case <-ticker.C:
			agent, done, err := server.checkChatContextReported(ctx, workspaceCtx, logger)
			if err != nil {
				return database.WorkspaceAgent{}, err
			}
			if done {
				return agent, nil
			}
		}
	}
}

// checkChatContextReported performs one context-gate poll: it re-resolves
// (and possibly rebinds) the chat's agent, applies the fast-fail conditions,
// and reports done=true once the bound agent has a pushed snapshot, at which
// point the chat is pinned to it. done=false with a nil error means "keep
// waiting".
func (server *Server) checkChatContextReported(
	ctx context.Context,
	workspaceCtx *turnWorkspaceContext,
	logger slog.Logger,
) (database.WorkspaceAgent, bool, error) {
	chatSnapshot, agent, err := workspaceCtx.refreshWorkspaceAgent(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return database.WorkspaceAgent{}, false, ctx.Err()
		}
		if !xerrors.Is(err, errChatHasNoWorkspaceAgent) {
			return database.WorkspaceAgent{}, false, err
		}
		// The latest build has no agent rows. A start build that is still
		// provisioning will grow them, so keep waiting; any other
		// transition never will, so fail fast.
		started, buildErr := server.latestBuildIsStart(ctx, chatSnapshot.WorkspaceID)
		if buildErr != nil {
			return database.WorkspaceAgent{}, false, buildErr
		}
		if !started {
			return database.WorkspaceAgent{}, false, terminalGeneration(errChatContextWorkspaceNotStarted)
		}
		return database.WorkspaceAgent{}, false, nil
	}

	// A resolved binding is kept as-is when the latest build is not a
	// start transition, so an unpinned chat on a stopped workspace would
	// otherwise wait the full ceiling for a push that cannot arrive.
	started, buildErr := server.latestBuildIsStart(ctx, chatSnapshot.WorkspaceID)
	if buildErr != nil {
		return database.WorkspaceAgent{}, false, buildErr
	}
	if !started {
		return database.WorkspaceAgent{}, false, terminalGeneration(errChatContextWorkspaceNotStarted)
	}

	if agentTooOldForContextReport(ctx, agent, logger) {
		return database.WorkspaceAgent{}, false, terminalGeneration(errChatContextAgentTooOld)
	}

	//nolint:gocritic // Chatd reads the agent's snapshot as the daemon subject.
	_, _, hasSnapshot, err := latestAgentSnapshot(dbauthz.AsChatd(ctx), server.db, agent.ID)
	if err != nil {
		return database.WorkspaceAgent{}, false, err
	}
	if !hasSnapshot {
		return database.WorkspaceAgent{}, false, nil
	}

	// The snapshot may have hydrated the chat mid-wait (pushes stamp bound
	// NULL-hash chats), so reload the row before pinning.
	fresh, err := workspaceCtx.loadChatSnapshot(ctx, chatSnapshot.ID)
	if err != nil {
		return database.WorkspaceAgent{}, false, xerrors.Errorf(
			"reload chat after context report: %w", err)
	}
	if !chatContextPinned(fresh) {
		if fresh.ContextAggregateHash == nil {
			// A never-pinned chat carries a NULL hash, which the push-side
			// NULL-gated hydrate can stamp; this also never clobbers a
			// concurrent push that hydrated the chat first.
			server.ensureChatContextPinnedOnFirstTurn(ctx, fresh)
		} else {
			// A rebind-cleared pin is an empty, non-NULL hash (the postgres
			// driver encodes the nil clear as an empty bytea), which the
			// NULL-gated hydrate skips. Re-pin explicitly, mirroring the
			// refresh endpoint.
			//nolint:gocritic // Chatd re-pins chats it does not own as the daemon subject.
			repinCtx := dbauthz.AsChatd(ctx)
			if err := database.ReadModifyUpdate(server.db, func(tx database.Store) error {
				return repinChatContext(repinCtx, tx, fresh.ID, fresh.AgentID)
			}); err != nil {
				return database.WorkspaceAgent{}, false, xerrors.Errorf(
					"pin chat to reported context: %w", err)
			}
		}
		fresh, err = workspaceCtx.loadChatSnapshot(ctx, chatSnapshot.ID)
		if err != nil {
			return database.WorkspaceAgent{}, false, xerrors.Errorf(
				"reload chat after context pin: %w", err)
		}
		// The pin committed here (not on the push path, whose own hydrate
		// publishes for the chats it stamps), so announce the freshly
		// pinned context to watchers.
		if chatContextPinned(fresh) {
			server.publishChatPubsubEvents(
				[]database.Chat{fresh}, codersdk.ChatWatchEventKindContextReady)
		}
	}
	workspaceCtx.setCurrentChat(fresh)
	return agent, true, nil
}

// chatContextPinned reports whether the chat carries a meaningful pinned
// context hash. A NULL hash (never hydrated) and an empty non-NULL hash (a
// rebind cleared the pin; the postgres driver stores the nil clear as an
// empty bytea) both count as unpinned.
func chatContextPinned(chat database.Chat) bool {
	return len(chat.ContextAggregateHash) > 0
}

// latestBuildIsStart reports whether the workspace's most recent build is a
// start transition.
func (server *Server) latestBuildIsStart(
	ctx context.Context,
	workspaceID uuid.NullUUID,
) (bool, error) {
	if !workspaceID.Valid {
		return false, xerrors.New("chat has no workspace")
	}
	build, err := server.db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID.UUID)
	if err != nil {
		return false, xerrors.Errorf("get latest workspace build: %w", err)
	}
	return build.Transition == database.WorkspaceTransitionStart, nil
}

// agentTooOldForContextReport reports whether the agent connected with an
// Agent API version below the context-push minimum. api_version is empty
// until the agent's first connect, so an agent that has never connected is
// not considered too old; the gate keeps waiting for it instead. An
// unparsable version is logged and treated the same way.
func agentTooOldForContextReport(
	ctx context.Context,
	agent database.WorkspaceAgent,
	logger slog.Logger,
) bool {
	if agent.APIVersion == "" {
		return false
	}
	major, minor, err := apiversion.Parse(agent.APIVersion)
	if err != nil {
		logger.Warn(ctx, "context gate: unparsable agent api version",
			slog.F("agent_id", agent.ID),
			slog.F("api_version", agent.APIVersion),
			slog.Error(err))
		return false
	}
	if major != contextReportMinAPIMajor {
		return major < contextReportMinAPIMajor
	}
	return minor < contextReportMinAPIMinor
}
