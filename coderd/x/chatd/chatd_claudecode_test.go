package chatd_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	acp "github.com/coder/acp-go-sdk"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chatd/claudecode"
	"github.com/coder/coder/v2/coderd/x/chatd/claudecode/claudecodetest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
	"github.com/coder/coder/v2/testutil"
)

// claudeCodeTestSetup carries the seeded rows shared by the runtime
// integration tests.
type claudeCodeTestSetup struct {
	user      database.User
	org       database.Organization
	workspace database.WorkspaceTable
	agent     database.WorkspaceAgent
}

// seedClaudeCodeChatDependencies seeds everything a claude_code chat
// turn needs: an org member, an enabled anthropic provider with a key,
// an enabled runtime config, and a workspace chain with an agent.
// Passing a stop transition leaves the latest build as a settled stop,
// exercising the restart path.
func seedClaudeCodeChatDependencies(t *testing.T, db database.Store, transition database.WorkspaceTransition) claudeCodeTestSetup {
	t.Helper()
	ctx := context.Background()

	user := dbgen.User(t, db, database.User{})
	_ = testAPIKeyID(t, db, user.ID)
	org := dbgen.Organization(t, db, database.Organization{})
	dbgen.OrganizationMember(t, db, database.OrganizationMember{
		UserID:         user.ID,
		OrganizationID: org.ID,
	})
	_ = dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    "anthropic",
		DisplayName: "anthropic",
		Enabled:     true,
	}, func(p *database.InsertChatProviderParams) {
		p.APIKey = "test-anthropic-key"
	})

	tv := dbgen.TemplateVersion(t, db, database.TemplateVersion{
		OrganizationID: org.ID,
		CreatedBy:      user.ID,
	})
	tpl := dbgen.Template(t, db, database.Template{
		CreatedBy:       user.ID,
		OrganizationID:  org.ID,
		ActiveVersionID: tv.ID,
	})
	ws := dbgen.Workspace(t, db, database.WorkspaceTable{
		TemplateID:     tpl.ID,
		OwnerID:        user.ID,
		OrganizationID: org.ID,
	})

	pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
		InitiatorID:    user.ID,
		OrganizationID: org.ID,
		StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
		CompletedAt:    sql.NullTime{Time: dbtime.Now(), Valid: true},
	})
	_ = dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
		TemplateVersionID: tv.ID,
		WorkspaceID:       ws.ID,
		JobID:             pj.ID,
		Transition:        transition,
	})
	res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
		Transition: transition,
		JobID:      pj.ID,
	})
	agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
		ResourceID:      res.ID,
		Directory:       "/home/coder/project",
		OperatingSystem: "linux",
	})
	require.NoError(t, db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
		ID:                agent.ID,
		Version:           "v1.0.0",
		ExpandedDirectory: "/home/coder/project",
	}))
	agent, err := db.GetWorkspaceAgentByID(ctx, agent.ID)
	require.NoError(t, err)

	_, err = db.UpsertChatRuntimeConfig(ctx, database.UpsertChatRuntimeConfigParams{
		OrganizationID: org.ID,
		Runtime:        database.ChatRuntimeClaudeCode,
		TemplateID:     tpl.ID,
		Enabled:        true,
		Model:          "claude-test-model",
		PermissionMode: "acceptEdits",
	})
	require.NoError(t, err)

	return claudeCodeTestSetup{
		user:      user,
		org:       org,
		workspace: ws,
		agent:     agent,
	}
}

func createClaudeCodeChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	ps pubsub.Pubsub,
	setup claudeCodeTestSetup,
	prompt string,
) chatstate.CreateChatResult {
	t.Helper()

	content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(prompt),
	})
	require.NoError(t, err)
	created, err := chatstate.CreateChat(ctx, db, ps, chatstate.CreateChatInput{
		OrganizationID: setup.org.ID,
		OwnerID:        setup.user.ID,
		WorkspaceID:    uuid.NullUUID{UUID: setup.workspace.ID, Valid: true},
		Runtime:        database.ChatRuntimeClaudeCode,
		Title:          "claude code chat",
		ClientType:     database.ChatClientTypeUi,
		InitialMessages: []chatstate.Message{
			{
				Role:           database.ChatMessageRoleUser,
				Content:        content,
				Visibility:     database.ChatMessageVisibilityBoth,
				ContentVersion: chatprompt.CurrentContentVersion,
				CreatedBy:      uuid.NullUUID{UUID: setup.user.ID, Valid: true},
			},
		},
	})
	require.NoError(t, err)
	return created
}

// claudeCodeConfigOverrides wires the fake ACP transport and a mocked
// workspace agent connection into the chatd server config.
func claudeCodeConfigOverrides(t *testing.T, agent *claudecodetest.FakeAgent) func(*chatd.Config) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	mockConn.EXPECT().SetExtraHeaders(gomock.Any()).AnyTimes()

	return func(cfg *chatd.Config) {
		cfg.AgentConn = func(_ context.Context, _ uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			return mockConn, func() {}, nil
		}
		cfg.ClaudeCodeTransport = func(_ context.Context, _ workspacesdk.AgentConn, _ database.WorkspaceAgent, env map[string]string) (claudecode.Transport, func(), error) {
			require.Equal(t, "test-anthropic-key", env["ANTHROPIC_API_KEY"])
			require.Equal(t, "claude-test-model", env["ANTHROPIC_MODEL"])
			return &claudecodetest.PipeTransport{Agent: agent}, func() {}, nil
		}
	}
}

func TestClaudeCodeChatTurn(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	setup := seedClaudeCodeChatDependencies(t, db, database.WorkspaceTransitionStart)

	fakeAgent := &claudecodetest.FakeAgent{}
	fakeAgent.OnPrompt = func(ctx context.Context, conn *acp.AgentSideConnection, params acp.PromptRequest) (acp.PromptResponse, error) {
		err := conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: params.SessionId,
			Update: acp.SessionUpdate{
				AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("Hello from Claude Code")},
			},
		})
		if err != nil {
			return acp.PromptResponse{}, err
		}
		return acp.PromptResponse{
			StopReason: acp.StopReasonEndTurn,
			Usage:      &acp.Usage{InputTokens: 11, OutputTokens: 7, TotalTokens: 18},
		}, nil
	}

	created := createClaudeCodeChat(ctx, t, db, ps, setup, "hello claude")
	_ = newActiveTestServer(t, db, ps, claudeCodeConfigOverrides(t, fakeAgent))

	chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chat.Status)
	require.False(t, chat.LastError.Valid)

	// The adapter session mode came from the runtime config.
	modes := fakeAgent.Modes()
	require.Len(t, modes, 1)
	require.Equal(t, acp.SessionModeId("acceptEdits"), modes[0].ModeId)

	// The prompt carried the user message text.
	prompts := fakeAgent.Prompts()
	require.Len(t, prompts, 1)
	require.Equal(t, "hello claude", prompts[0].Prompt[0].Text.Text)
	// The session cwd came from the agent's expanded directory.
	sessions := fakeAgent.NewSessions()
	require.Len(t, sessions, 1)
	require.Equal(t, "/home/coder/project", sessions[0].Cwd)

	// The assistant reply was committed with usage tokens.
	messages, err := db.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{ChatID: created.Chat.ID})
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, database.ChatMessageRoleAssistant, messages[1].Role)
	parts, err := chatprompt.ParseContent(messages[1])
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, codersdk.ChatMessagePartTypeText, parts[0].Type)
	require.Equal(t, "Hello from Claude Code", parts[0].Text)
	require.Equal(t, int64(11), messages[1].InputTokens.Int64)
	require.Equal(t, int64(7), messages[1].OutputTokens.Int64)

	// The ACP session was persisted for the next turn to resume.
	state := claudecode.ParseRuntimeState(chat.RuntimeState.RawMessage)
	require.Equal(t, "session-new", state.SessionID)
	require.Equal(t, "/home/coder/project", state.Cwd)
}

func TestClaudeCodeChatResumesSession(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	setup := seedClaudeCodeChatDependencies(t, db, database.WorkspaceTransitionStart)

	fakeAgent := &claudecodetest.FakeAgent{
		Capabilities: acp.AgentCapabilities{
			SessionCapabilities: acp.SessionCapabilities{Resume: &acp.SessionResumeCapabilities{}},
		},
	}

	created := createClaudeCodeChat(ctx, t, db, ps, setup, "first message")
	server := newActiveTestServer(t, db, ps, claudeCodeConfigOverrides(t, fakeAgent))

	chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chat.Status)
	require.Len(t, fakeAgent.NewSessions(), 1)

	// Second turn: a fresh adapter process must resume the recorded
	// session instead of starting over.
	_, err := server.SendMessage(ctx, chatd.SendMessageOptions{
		ChatID:    chat.ID,
		CreatedBy: setup.user.ID,
		APIKeyID:  testAPIKeyID(t, db, setup.user.ID),
		Content: []codersdk.ChatMessagePart{
			codersdk.ChatMessageText("second message"),
		},
	})
	require.NoError(t, err)

	testutil.Eventually(ctx, t, func(ctx context.Context) bool {
		got, err := db.GetChatByID(ctx, chat.ID)
		if err != nil {
			return false
		}
		return got.Status == database.ChatStatusWaiting && got.HistoryVersion > chat.HistoryVersion
	}, testutil.IntervalFast)

	resumes := fakeAgent.ResumeSessions()
	require.Len(t, resumes, 1)
	require.Equal(t, acp.SessionId("session-new"), resumes[0].SessionId)
	require.Equal(t, "/home/coder/project", resumes[0].Cwd)
	// Still exactly one session/new: the second turn resumed.
	require.Len(t, fakeAgent.NewSessions(), 1)
}

func TestClaudeCodeChatRestartsStoppedWorkspace(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	setup := seedClaudeCodeChatDependencies(t, db, database.WorkspaceTransitionStop)

	fakeAgent := &claudecodetest.FakeAgent{}
	created := createClaudeCodeChat(ctx, t, db, ps, setup, "wake up")

	overrides := claudeCodeConfigOverrides(t, fakeAgent)
	_ = newActiveTestServer(t, db, ps, func(cfg *chatd.Config) {
		overrides(cfg)
		cfg.StartWorkspace = func(ctx context.Context, ownerID uuid.UUID, workspaceID uuid.UUID, req codersdk.CreateWorkspaceBuildRequest) (codersdk.WorkspaceBuild, error) {
			require.Equal(t, setup.user.ID, ownerID)
			require.Equal(t, setup.workspace.ID, workspaceID)
			require.Equal(t, codersdk.WorkspaceTransitionStart, req.Transition)
			// Simulate the provisioner completing a start build.
			build, err := db.GetLatestWorkspaceBuildByWorkspaceID(ctx, workspaceID)
			if err != nil {
				return codersdk.WorkspaceBuild{}, err
			}
			pj := dbgen.ProvisionerJob(t, db, nil, database.ProvisionerJob{
				InitiatorID:    ownerID,
				OrganizationID: setup.org.ID,
				StartedAt:      sql.NullTime{Time: dbtime.Now(), Valid: true},
				CompletedAt:    sql.NullTime{Time: dbtime.Now(), Valid: true},
			})
			newBuild := dbgen.WorkspaceBuild(t, db, database.WorkspaceBuild{
				TemplateVersionID: build.TemplateVersionID,
				WorkspaceID:       workspaceID,
				JobID:             pj.ID,
				Transition:        database.WorkspaceTransitionStart,
				BuildNumber:       build.BuildNumber + 1,
			})
			res := dbgen.WorkspaceResource(t, db, database.WorkspaceResource{
				Transition: database.WorkspaceTransitionStart,
				JobID:      pj.ID,
			})
			agent := dbgen.WorkspaceAgent(t, db, database.WorkspaceAgent{
				ResourceID:      res.ID,
				Directory:       "/home/coder/project",
				OperatingSystem: "linux",
			})
			if err := db.UpdateWorkspaceAgentStartupByID(ctx, database.UpdateWorkspaceAgentStartupByIDParams{
				ID:                agent.ID,
				Version:           "v1.0.0",
				ExpandedDirectory: "/home/coder/project",
			}); err != nil {
				return codersdk.WorkspaceBuild{}, err
			}
			return codersdk.WorkspaceBuild{ID: newBuild.ID}, nil
		}
	})

	chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
	require.Equal(t, database.ChatStatusWaiting, chat.Status)
	require.False(t, chat.LastError.Valid)
	require.Len(t, fakeAgent.Prompts(), 1)
}

func TestClaudeCodeChatMissingRuntimeConfigFails(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	ctx := testutil.Context(t, testutil.WaitLong)

	setup := seedClaudeCodeChatDependencies(t, db, database.WorkspaceTransitionStart)
	require.NoError(t, db.DeleteChatRuntimeConfig(ctx, database.DeleteChatRuntimeConfigParams{
		OrganizationID: setup.org.ID,
		Runtime:        database.ChatRuntimeClaudeCode,
	}))

	fakeAgent := &claudecodetest.FakeAgent{}
	created := createClaudeCodeChat(ctx, t, db, ps, setup, "hello")
	_ = newActiveTestServer(t, db, ps, claudeCodeConfigOverrides(t, fakeAgent))

	chat := waitForTerminalChat(ctx, t, db, created.Chat.ID)
	require.Equal(t, database.ChatStatusError, chat.Status)
	require.True(t, chat.LastError.Valid)
	require.Contains(t, string(chat.LastError.RawMessage), "not configured")
	require.Empty(t, fakeAgent.Prompts())
}
