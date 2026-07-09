package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/coderd/x/chatd/claudecode"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/quartz"
)

const (
	// claudeCodeWorkspaceReadyTimeout bounds the whole workspace
	// readiness phase of a turn: waiting for an in-flight build,
	// starting a stopped workspace, and dialing the agent.
	claudeCodeWorkspaceReadyTimeout = 10 * time.Minute
	// claudeCodeWorkspacePollInterval paces build and dial polling.
	claudeCodeWorkspacePollInterval = 3 * time.Second
	// claudeCodePreflightTimeout bounds the adapter binary check.
	claudeCodePreflightTimeout = 30 * time.Second
	// claudeCodePersistStateTimeout bounds the best-effort
	// runtime_state write after a turn.
	claudeCodePersistStateTimeout = 15 * time.Second
)

// startClaudeCodeGeneration runs one claude_code runtime turn. The
// whole turn (workspace readiness, ACP prompt, tool execution inside
// the workspace) happens in a single generation action; the built-in
// prepare/decide loop does not apply. The function is re-entrant: when
// the turn's output is already committed (the runner re-dispatches
// after every commit, or a previous worker crashed between commit and
// finish), it finishes the turn instead of prompting again.
func (s *taskStarter) startClaudeCodeGeneration(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	input chatWorkerTaskStartInput,
	chat database.Chat,
	history []database.ChatMessage,
) error {
	turn, err := claudeCodeTurnFromHistory(ctx, s.opts.Logger, history)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}
	if !turn.generate {
		return s.finishGenerationTurn(ctx, machine, input, generationDecision{
			kind:         generationActionFinishTurn,
			finishReason: generationFinishReasonComplete,
		}, generationAttemptNotRequired)
	}

	cfg, err := s.server.claudeCodeTurnConfig(ctx, chat)
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}

	if err := s.ensureClaudeCodeWorkspaceRunning(ctx, chat); err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("ensure workspace running: %w", err))
		}
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}

	currentChat := chat
	var chatStateMu sync.Mutex
	workspaceCtx := turnWorkspaceContext{
		server:      s.server,
		chatStateMu: &chatStateMu,
		currentChat: &currentChat,
		loadChatSnapshot: func(loadCtx context.Context, chatID uuid.UUID) (database.Chat, error) {
			return s.server.db.GetChatByID(loadCtx, chatID)
		},
	}
	defer workspaceCtx.close()

	conn, agent, err := s.dialClaudeCodeAgent(ctx, &workspaceCtx)
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("dial workspace agent: %w", err))
		}
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}

	env := map[string]string{
		"ANTHROPIC_API_KEY": cfg.apiKey,
	}
	if cfg.model != "" {
		env["ANTHROPIC_MODEL"] = cfg.model
	}
	if cfg.baseURL != "" {
		env["ANTHROPIC_BASE_URL"] = cfg.baseURL
	}

	transportFn := s.server.claudeCodeTransportFn
	if transportFn == nil {
		transportFn = s.sshClaudeCodeTransport
	}
	transport, closeTransport, err := transportFn(ctx, conn, env)
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(errTaskExpectedExit, xerrors.Errorf("claude code transport: %w", err))
		}
		return s.finishGenerationError(ctx, machine, input, err, generationAttemptNotRequired)
	}
	defer closeTransport()

	cwd := agent.ExpandedDirectory
	if cwd == "" {
		cwd, err = chattool.ResolveWorkspaceHome(ctx, conn)
		if err != nil {
			return s.finishGenerationError(ctx, machine, input, xerrors.Errorf("resolve workspace home: %w", err), generationAttemptNotRequired)
		}
	}

	attempt, err := s.beginGenerationAttempt(ctx, machine, input)
	if err != nil {
		return xerrors.Errorf("begin generation attempt: %w", err)
	}
	defer attempt.closeEpisode()

	state := claudecode.ParseRuntimeState(chat.RuntimeState.RawMessage)
	startedAt := s.opts.Clock.Now("chatworker", "claudecode")
	outcome, runErr := claudecode.RunTurn(ctx, transport, claudecode.TurnInput{
		SessionID:      state.SessionID,
		SessionCwd:     state.Cwd,
		Cwd:            cwd,
		PromptText:     turn.prompt,
		ReseedContext:  claudecode.BuildReseedContext(turn.reseed),
		PermissionMode: cfg.permissionMode,
		Publish:        attempt.publish,
		Logger:         s.opts.Logger,
	})
	elapsed := s.opts.Clock.Now("chatworker", "claudecode").Sub(startedAt)

	turnUsage, usageTotals := claudeCodeTurnUsage(outcome, state)

	// Record the session even when the turn was interrupted or the
	// commit below fails: the workspace-side session advanced either
	// way, and the next turn should resume it.
	if outcome.SessionID != "" {
		s.persistClaudeCodeRuntimeState(ctx, chat.ID, outcome, state, cwd, usageTotals)
	}

	if ctx.Err() != nil {
		// Interrupted: the interrupt task persists buffered partials.
		return errors.Join(errTaskExpectedExit, xerrors.Errorf("claude code turn interrupted: %w", ctx.Err()))
	}
	if runErr != nil {
		return s.finishGenerationError(ctx, machine, input, runErr, requireGenerationAttempt(attempt.number))
	}
	if len(outcome.Content) == 0 {
		return s.finishGenerationTurn(ctx, machine, input, generationDecision{
			kind:         generationActionFinishTurn,
			finishReason: generationFinishReasonComplete,
		}, requireGenerationAttempt(attempt.number))
	}

	messages, err := buildCommitStepMessages(buildCommitStepMessagesInput{
		step: stepData{
			Content: outcome.Content,
			Usage:   turnUsage,
			Runtime: elapsed,
		},
		logger:         s.opts.Logger,
		contentVersion: chatprompt.CurrentContentVersion,
	})
	if err != nil {
		return s.finishGenerationError(ctx, machine, input, err, requireGenerationAttempt(attempt.number))
	}
	return s.commitGenerationStep(ctx, machine, input, attempt.number, generationActionGenerateAssistant, messages)
}

type claudeCodeTurn struct {
	generate bool
	prompt   string
	reseed   []claudecode.ReseedTurn
}

// claudeCodeTurnFromHistory derives the ACP prompt for this turn from
// persisted history. The prompt is the trailing run of user messages
// (multiple when earlier turns failed before generating a reply);
// everything before it becomes reseed context for the lossy
// new-session fallback. generate is false when history ends with
// assistant or tool output, meaning the turn already generated and
// only FinishTurn remains.
func claudeCodeTurnFromHistory(ctx context.Context, logger slog.Logger, history []database.ChatMessage) (claudeCodeTurn, error) {
	firstTrailingUser := len(history)
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != database.ChatMessageRoleUser {
			break
		}
		firstTrailingUser = i
	}
	if firstTrailingUser == len(history) {
		return claudeCodeTurn{}, nil
	}

	var prompt strings.Builder
	for _, msg := range history[firstTrailingUser:] {
		text, err := chatMessageText(msg)
		if err != nil {
			return claudeCodeTurn{}, xerrors.Errorf("parse user message %d: %w", msg.ID, err)
		}
		if text == "" {
			continue
		}
		if prompt.Len() > 0 {
			_, _ = prompt.WriteString("\n\n")
		}
		_, _ = prompt.WriteString(text)
	}
	if strings.TrimSpace(prompt.String()) == "" {
		return claudeCodeTurn{}, chaterror.WithClassification(
			xerrors.New("user message has no text content"),
			chaterror.ClassifiedError{
				Kind:    codersdk.ChatErrorKindGeneric,
				Message: "Claude Code chats currently support text messages only.",
			},
		)
	}

	reseed := make([]claudecode.ReseedTurn, 0, firstTrailingUser)
	for _, msg := range history[:firstTrailingUser] {
		var role string
		switch msg.Role {
		case database.ChatMessageRoleUser:
			role = "User"
		case database.ChatMessageRoleAssistant:
			role = "Assistant"
		default:
			continue
		}
		text, err := chatMessageText(msg)
		if err != nil {
			// Reseed is lossy by design; skip entries that fail to
			// parse instead of failing the turn.
			logger.Debug(ctx, "skip reseed message", slog.F("message_id", msg.ID), slog.Error(err))
			continue
		}
		if text == "" {
			continue
		}
		reseed = append(reseed, claudecode.ReseedTurn{Role: role, Text: text})
	}

	return claudeCodeTurn{
		generate: true,
		prompt:   prompt.String(),
		reseed:   reseed,
	}, nil
}

// chatMessageText joins the text parts of a persisted message.
func chatMessageText(msg database.ChatMessage) (string, error) {
	parts, err := chatprompt.ParseContent(msg)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, part := range parts {
		if part.Type != codersdk.ChatMessagePartTypeText || part.Text == "" {
			continue
		}
		if sb.Len() > 0 {
			_, _ = sb.WriteString("\n\n")
		}
		_, _ = sb.WriteString(part.Text)
	}
	return sb.String(), nil
}

type claudeCodeTurnConfig struct {
	model          string
	permissionMode string
	apiKey         string
	baseURL        string
}

// claudeCodeTurnConfig loads the organization's runtime config and the
// deployment Anthropic key for one turn. The key is injected into the
// adapter's process environment and never written to workspace disk.
func (p *Server) claudeCodeTurnConfig(ctx context.Context, chat database.Chat) (claudeCodeTurnConfig, error) {
	cfg, err := p.db.GetChatRuntimeConfig(ctx, database.GetChatRuntimeConfigParams{
		OrganizationID: chat.OrganizationID,
		Runtime:        chat.Runtime,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return claudeCodeTurnConfig{}, chaterror.WithClassification(
				xerrors.New("chat runtime config missing"),
				chaterror.ClassifiedError{
					Kind:    codersdk.ChatErrorKindConfig,
					Message: "The Claude Code runtime is not configured for this organization.",
				},
			)
		}
		return claudeCodeTurnConfig{}, xerrors.Errorf("get chat runtime config: %w", err)
	}
	if !cfg.Enabled {
		return claudeCodeTurnConfig{}, chaterror.WithClassification(
			xerrors.New("chat runtime config disabled"),
			chaterror.ClassifiedError{
				Kind:    codersdk.ChatErrorKindProviderDisabled,
				Message: "The Claude Code runtime is disabled for this organization.",
			},
		)
	}

	providers, err := p.db.GetAIProviders(ctx, database.GetAIProvidersParams{})
	if err != nil {
		return claudeCodeTurnConfig{}, xerrors.Errorf("get ai providers: %w", err)
	}
	for _, provider := range providers {
		if provider.Type != database.AIProviderTypeAnthropic {
			continue
		}
		configured, err := p.aiProviderConfig(ctx, provider)
		if err != nil {
			p.logger.Warn(ctx, "claude code turn: load anthropic provider config",
				slog.F("provider_id", provider.ID), slog.Error(err))
			continue
		}
		if configured.APIKey == "" {
			continue
		}
		return claudeCodeTurnConfig{
			model:          cfg.Model,
			permissionMode: cfg.PermissionMode,
			apiKey:         configured.APIKey,
			baseURL:        configured.BaseURL,
		}, nil
	}
	return claudeCodeTurnConfig{}, chaterror.WithClassification(
		xerrors.New("no anthropic provider key configured"),
		chaterror.ClassifiedError{
			Kind:    codersdk.ChatErrorKindMissingKey,
			Message: "Claude Code requires a deployment Anthropic API key. An administrator must configure the Anthropic AI provider.",
		},
	)
}

// ensureClaudeCodeWorkspaceRunning makes sure the chat's bound
// workspace has a succeeded start build, creating one as the chat
// owner when the workspace is stopped. Agent reachability is handled
// by the dial loop afterwards.
func (s *taskStarter) ensureClaudeCodeWorkspaceRunning(ctx context.Context, chat database.Chat) error {
	if !chat.WorkspaceID.Valid {
		return chaterror.WithClassification(
			xerrors.New("claude code chat has no bound workspace"),
			chaterror.ClassifiedError{
				Kind:    codersdk.ChatErrorKindConfig,
				Message: "This chat has no workspace bound to it, so the Claude Code runtime cannot run.",
			},
		)
	}
	db := s.server.db
	workspace, err := db.GetWorkspaceByID(ctx, chat.WorkspaceID.UUID)
	if err != nil {
		return xerrors.Errorf("get workspace: %w", err)
	}

	deletedErr := chaterror.WithClassification(
		xerrors.New("chat workspace deleted"),
		chaterror.ClassifiedError{
			Kind:    codersdk.ChatErrorKindConfig,
			Message: "The workspace backing this chat was deleted, so the conversation cannot continue.",
		},
	)
	if workspace.Deleted {
		return deletedErr
	}

	deadline := s.opts.Clock.Now("chatworker", "claudecode-workspace").Add(claudeCodeWorkspaceReadyTimeout)
	started := false
	for {
		build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspace.ID)
		if err != nil {
			return xerrors.Errorf("get latest workspace build: %w", err)
		}
		job, err := db.GetProvisionerJobByID(ctx, build.JobID)
		if err != nil {
			return xerrors.Errorf("get workspace build job: %w", err)
		}
		switch {
		case build.Transition == database.WorkspaceTransitionDelete:
			return deletedErr
		case job.JobStatus == database.ProvisionerJobStatusPending || job.JobStatus == database.ProvisionerJobStatusRunning:
			// A build is in flight (either direction); wait for it to
			// settle before deciding whether to start.
		case build.Transition == database.WorkspaceTransitionStart && job.JobStatus == database.ProvisionerJobStatusSucceeded:
			return nil
		default:
			// The latest build is a settled stop, or a failed or
			// canceled build.
			if started {
				return chaterror.WithClassification(
					xerrors.New("workspace start build did not succeed"),
					chaterror.ClassifiedError{
						Kind:    codersdk.ChatErrorKindGeneric,
						Message: "The workspace backing this chat failed to start. Check the workspace build logs.",
					},
				)
			}
			if s.server.startWorkspaceFn == nil {
				return xerrors.New("workspace starting is not configured")
			}
			s.opts.Logger.Info(ctx, "starting stopped workspace for claude code chat",
				slog.F("chat_id", chat.ID), slog.F("workspace_id", workspace.ID))
			if _, err := s.server.startWorkspaceFn(ctx, chat.OwnerID, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
			}); err != nil {
				return chaterror.WithClassification(
					xerrors.Errorf("start workspace: %w", err),
					chaterror.ClassifiedError{
						Kind:    codersdk.ChatErrorKindGeneric,
						Message: "The workspace backing this chat could not be started.",
					},
				)
			}
			started = true
		}
		if !s.opts.Clock.Now("chatworker", "claudecode-workspace").Before(deadline) {
			return chaterror.WithClassification(
				xerrors.New("timed out waiting for workspace to start"),
				chaterror.ClassifiedError{
					Kind:    codersdk.ChatErrorKindTimeout,
					Message: "Timed out waiting for the workspace backing this chat to start.",
				},
			)
		}
		timer := s.opts.Clock.NewTimer(claudeCodeWorkspacePollInterval, "chatworker", "claudecode-workspace")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return xerrors.Errorf("wait for workspace: %w", ctx.Err())
		}
	}
}

// dialClaudeCodeAgent dials the chat's workspace agent, retrying while
// the agent connects after a workspace start. The turnWorkspaceContext
// handles agent selection, chat binding persistence, and lazy
// validation.
func (s *taskStarter) dialClaudeCodeAgent(
	ctx context.Context,
	workspaceCtx *turnWorkspaceContext,
) (workspacesdk.AgentConn, database.WorkspaceAgent, error) {
	deadline := s.opts.Clock.Now("chatworker", "claudecode-dial").Add(claudeCodeWorkspaceReadyTimeout)
	for {
		conn, dialErr := workspaceCtx.getWorkspaceConn(ctx)
		if dialErr == nil {
			_, agent, err := workspaceCtx.ensureWorkspaceAgent(ctx)
			if err != nil {
				return nil, database.WorkspaceAgent{}, xerrors.Errorf("resolve workspace agent: %w", err)
			}
			return conn, agent, nil
		}
		if ctx.Err() != nil {
			return nil, database.WorkspaceAgent{}, xerrors.Errorf("dial workspace agent: %w", errors.Join(dialErr, ctx.Err()))
		}
		if !s.opts.Clock.Now("chatworker", "claudecode-dial").Before(deadline) {
			return nil, database.WorkspaceAgent{}, chaterror.WithClassification(
				xerrors.Errorf("dial workspace agent: %w", dialErr),
				chaterror.ClassifiedError{
					Kind:    codersdk.ChatErrorKindTimeout,
					Message: "Timed out waiting for the workspace agent to become reachable.",
				},
			)
		}
		timer := s.opts.Clock.NewTimer(claudeCodeWorkspacePollInterval, "chatworker", "claudecode-dial")
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, database.WorkspaceAgent{}, xerrors.Errorf("dial workspace agent: %w", ctx.Err())
		}
	}
}

// ClaudeCodeTransportFunc builds the ACP transport for one turn from
// an established workspace agent connection. It exists as a seam so
// tests can substitute an in-memory transport; production uses
// sshClaudeCodeTransport.
type ClaudeCodeTransportFunc func(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	env map[string]string,
) (claudecode.Transport, func(), error)

// sshClaudeCodeTransport opens an SSH client to the workspace agent,
// verifies the adapter binary exists, and returns the non-PTY SSH exec
// transport the ACP session runs over.
func (s *taskStarter) sshClaudeCodeTransport(
	ctx context.Context,
	conn workspacesdk.AgentConn,
	env map[string]string,
) (claudecode.Transport, func(), error) {
	sshClient, err := conn.SSHClient(ctx)
	if err != nil {
		return nil, nil, xerrors.Errorf("workspace ssh client: %w", err)
	}
	if err := claudeCodeAdapterPreflight(ctx, s.opts.Clock, sshClient); err != nil {
		_ = sshClient.Close()
		return nil, nil, err
	}
	return &claudecode.SSHTransport{
			Client: sshClient,
			Env:    env,
		}, func() {
			_ = sshClient.Close()
		}, nil
}

// claudeCodeAdapterPreflight verifies the adapter binary exists inside
// the workspace before starting a turn, so a template that does not
// ship it produces a legible configuration error instead of an opaque
// protocol failure.
func claudeCodeAdapterPreflight(ctx context.Context, clock quartz.Clock, client *gossh.Client) error {
	session, err := client.NewSession()
	if err != nil {
		return xerrors.Errorf("new ssh session: %w", err)
	}
	defer session.Close()

	done := make(chan error, 1)
	go func() {
		done <- session.Run("command -v " + claudecode.DefaultAdapterCommand)
	}()
	timer := clock.NewTimer(claudeCodePreflightTimeout, "chatworker", "claudecode-preflight")
	defer timer.Stop()
	select {
	case err = <-done:
	case <-timer.C:
		return xerrors.New("claude code adapter preflight timed out")
	case <-ctx.Done():
		return xerrors.Errorf("claude code adapter preflight: %w", ctx.Err())
	}
	if err != nil {
		return chaterror.WithClassification(
			xerrors.Errorf("claude code adapter preflight: %w", err),
			chaterror.ClassifiedError{
				Kind: codersdk.ChatErrorKindConfig,
				Message: "The workspace does not provide the Claude Code adapter (" + claudecode.DefaultAdapterCommand + "). " +
					"The template configured for this runtime must preinstall it.",
			},
		)
	}
	return nil
}

// persistClaudeCodeRuntimeState best-effort records the ACP session
// that served the turn so the next turn can resume it. It runs even
// when the turn was interrupted or the commit fails, because the
// workspace-side session advanced regardless.
func (s *taskStarter) persistClaudeCodeRuntimeState(
	ctx context.Context,
	chatID uuid.UUID,
	outcome claudecode.TurnOutcome,
	prior claudecode.RuntimeState,
	newCwd string,
	usageTotals *claudecode.UsageTotals,
) {
	cwd := newCwd
	if outcome.Resumed && prior.Cwd != "" {
		cwd = prior.Cwd
	}
	encoded, err := json.Marshal(claudecode.RuntimeState{
		SessionID: outcome.SessionID,
		Cwd:       cwd,
		Usage:     usageTotals,
		UpdatedAt: s.opts.Clock.Now("chatworker", "claudecode").UTC(),
	})
	if err != nil {
		s.opts.Logger.Warn(ctx, "marshal claude code runtime state", slog.F("chat_id", chatID), slog.Error(err))
		return
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), claudeCodePersistStateTimeout)
	defer cancel()
	if err := s.opts.Store.UpdateChatRuntimeState(persistCtx, database.UpdateChatRuntimeStateParams{
		ID:           chatID,
		RuntimeState: pqtype.NullRawMessage{RawMessage: encoded, Valid: true},
	}); err != nil {
		s.opts.Logger.Warn(persistCtx, "persist claude code runtime state", slog.F("chat_id", chatID), slog.Error(err))
	}
}

// claudeCodeTurnUsage derives per-turn token usage from ACP's
// session-cumulative counters: on a resumed session it subtracts the
// totals persisted after the previous turn, otherwise the session is
// fresh and the counters already are per-turn. It returns the new
// cumulative totals to persist alongside the session.
func claudeCodeTurnUsage(
	outcome claudecode.TurnOutcome,
	prior claudecode.RuntimeState,
) (fantasy.Usage, *claudecode.UsageTotals) {
	if outcome.Usage == nil {
		// No usage this turn: carry prior totals forward so a later
		// turn that does report usage still subtracts them.
		if outcome.Resumed && outcome.SessionID == prior.SessionID {
			return fantasy.Usage{}, prior.Usage
		}
		return fantasy.Usage{}, nil
	}
	totals := &claudecode.UsageTotals{
		InputTokens:  int64(outcome.Usage.InputTokens),
		OutputTokens: int64(outcome.Usage.OutputTokens),
		TotalTokens:  int64(outcome.Usage.TotalTokens),
	}
	if outcome.Usage.ThoughtTokens != nil {
		totals.ReasoningTokens = int64(*outcome.Usage.ThoughtTokens)
	}
	if outcome.Usage.CachedWriteTokens != nil {
		totals.CacheCreationTokens = int64(*outcome.Usage.CachedWriteTokens)
	}
	if outcome.Usage.CachedReadTokens != nil {
		totals.CacheReadTokens = int64(*outcome.Usage.CachedReadTokens)
	}
	base := claudecode.UsageTotals{}
	if outcome.Resumed && outcome.SessionID == prior.SessionID && prior.Usage != nil {
		base = *prior.Usage
	}
	usage := fantasy.Usage{
		InputTokens:         totals.InputTokens - base.InputTokens,
		OutputTokens:        totals.OutputTokens - base.OutputTokens,
		TotalTokens:         totals.TotalTokens - base.TotalTokens,
		ReasoningTokens:     totals.ReasoningTokens - base.ReasoningTokens,
		CacheCreationTokens: totals.CacheCreationTokens - base.CacheCreationTokens,
		CacheReadTokens:     totals.CacheReadTokens - base.CacheReadTokens,
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.TotalTokens < 0 ||
		usage.ReasoningTokens < 0 || usage.CacheCreationTokens < 0 || usage.CacheReadTokens < 0 {
		// A negative delta means the adapter restarted its counters;
		// the raw counts are then the closest per-turn approximation.
		usage = fantasy.Usage{
			InputTokens:         totals.InputTokens,
			OutputTokens:        totals.OutputTokens,
			TotalTokens:         totals.TotalTokens,
			ReasoningTokens:     totals.ReasoningTokens,
			CacheCreationTokens: totals.CacheCreationTokens,
			CacheReadTokens:     totals.CacheReadTokens,
		}
	}
	return usage, totals
}
