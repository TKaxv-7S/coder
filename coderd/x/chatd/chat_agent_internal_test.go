package chatd

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/codersdk"
)

// messageTexts returns the concatenated text parts of each message in
// history order.
func messageTexts(t *testing.T, msgs []database.ChatMessage) []string {
	t.Helper()
	texts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		parts, err := chatprompt.ParseContent(msg)
		require.NoError(t, err)
		var sb strings.Builder
		for _, part := range parts {
			if part.Type == codersdk.ChatMessagePartTypeText {
				_, _ = sb.WriteString(part.Text)
			}
		}
		texts = append(texts, sb.String())
	}
	return texts
}

// TestCreateChatWithPersona verifies that an active persona replaces
// DefaultSystemPrompt as the base of the first system message and that
// the agent prompt append is inserted as the following system message.
func TestCreateChatWithPersona(t *testing.T) {
	t.Parallel()

	const (
		personaPrompt = "You are the docs persona. Write clear documentation."
		promptAppend  = "Always include usage examples."
	)

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	persona := dbgen.ChatPersona(t, db, database.ChatPersona{
		Slug:         "docs-persona",
		SystemPrompt: personaPrompt,
		CreatedBy:    user.ID,
	})
	agent := dbgen.ChatAgent(t, db, database.ChatAgent{
		Slug:         "docs-agent",
		PersonaID:    persona.ID,
		PromptAppend: promptAppend,
		CreatedBy:    user.ID,
	})

	chat, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:      org.ID,
		OwnerID:             user.ID,
		APIKeyID:            testAPIKeyID(t, db, user.ID),
		Title:               "persona chat",
		ModelConfigID:       model.ID,
		InitialUserContent:  []codersdk.ChatMessagePart{codersdk.ChatMessageText("write docs")},
		ChatAgentID:         uuid.NullUUID{UUID: agent.ID, Valid: true},
		PersonaSystemPrompt: persona.SystemPrompt,
		AgentPromptAppend:   agent.PromptAppend,
	})
	require.NoError(t, err)
	require.True(t, chat.ChatAgentID.Valid)
	require.Equal(t, agent.ID, chat.ChatAgentID.UUID)

	msgs, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	texts := messageTexts(t, msgs)
	require.GreaterOrEqual(t, len(texts), 4)

	// The persona prompt replaces DefaultSystemPrompt as the base of
	// the first system message.
	require.Equal(t, personaPrompt, texts[0])
	require.NotContains(t, texts[0], "You are the Coder agent")
	// The agent prompt append follows as its own system message.
	require.Equal(t, promptAppend, texts[1])
}

// TestCreateChatWithoutPersonaKeepsDefault verifies that chats created
// without a persona keep today's DefaultSystemPrompt behavior exactly.
func TestCreateChatWithoutPersonaKeepsDefault(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)

	chat, err := server.CreateChat(ctx, CreateOptions{
		OrganizationID:     org.ID,
		OwnerID:            user.ID,
		APIKeyID:           testAPIKeyID(t, db, user.ID),
		Title:              "default chat",
		ModelConfigID:      model.ID,
		InitialUserContent: []codersdk.ChatMessagePart{codersdk.ChatMessageText("hello")},
	})
	require.NoError(t, err)
	require.False(t, chat.ChatAgentID.Valid)

	msgs, err := db.GetChatMessagesForPromptByChatID(ctx, chat.ID)
	require.NoError(t, err)
	texts := messageTexts(t, msgs)
	require.NotEmpty(t, texts)
	require.Contains(t, texts[0], "You are the Coder agent")
}

// TestSpawnAgentChatAgentType verifies that spawn_agent accepts
// agent:<slug> types and creates the child chat with the agent's
// persona prompt, prompt append, chat_agent_id, and model override.
func TestSpawnAgentChatAgentType(t *testing.T) {
	t.Parallel()

	const (
		personaPrompt = "You are the reviewer persona for spawn tests."
		promptAppend  = "Report findings as a numbered list."
	)

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	overrideModel := insertInternalChatModelConfig(
		t, db, "agent-override-"+uuid.NewString(), true,
	)
	persona := dbgen.ChatPersona(t, db, database.ChatPersona{
		Slug:         "spawn-persona",
		SystemPrompt: personaPrompt,
		CreatedBy:    user.ID,
	})
	agent := dbgen.ChatAgent(t, db, database.ChatAgent{
		OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
		Slug:           "spawn-agent",
		PersonaID:      persona.ID,
		PromptAppend:   promptAppend,
		ModelConfigID:  uuid.NullUUID{UUID: overrideModel.ID, Valid: true},
		CreatedBy:      user.ID,
	})

	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-agent-spawn",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeChatAgentPrefix + agent.Slug,
		Prompt: "review this change",
	})
	childID := requireSpawnAgentChildChatID(t, resp)

	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)
	require.True(t, childChat.ChatAgentID.Valid)
	require.Equal(t, agent.ID, childChat.ChatAgentID.UUID)
	require.Equal(t, overrideModel.ID, childChat.LastModelConfigID)

	msgs, err := db.GetChatMessagesForPromptByChatID(ctx, childID)
	require.NoError(t, err)
	texts := messageTexts(t, msgs)
	require.GreaterOrEqual(t, len(texts), 4)
	require.Equal(t, personaPrompt, texts[0])
	require.Equal(t, promptAppend, texts[1])
}

// TestSpawnAgentChatAgentTypeRejections verifies that unknown,
// disabled, and foreign-organization agent slugs are rejected with a
// tool error response.
func TestSpawnAgentChatAgentTypeRejections(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	otherOrg := dbgen.Organization(t, db, database.Organization{})

	disabledAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
		Slug:      "disabled-spawn-agent",
		CreatedBy: user.ID,
	}, func(params *database.InsertChatAgentParams) {
		params.Enabled = false
	})

	foreignAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
		OrganizationID: uuid.NullUUID{UUID: otherOrg.ID, Valid: true},
		Slug:           "foreign-spawn-agent",
		CreatedBy:      user.ID,
	})

	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-agent-rejections",
	)
	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)

	for _, slug := range []string{"no-such-agent", disabledAgent.Slug, foreignAgent.Slug} {
		resp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
			Type:   subagentTypeChatAgentPrefix + slug,
			Prompt: "should fail",
		})
		require.True(t, resp.IsError, "slug %q should be rejected, got: %s", slug, resp.Content)
		require.Contains(t, resp.Content, slug)
	}
}

// TestSpawnAgentChatAgentTypeRootOnly verifies that child chats cannot
// spawn agent:<slug> children, matching the builtin type guardrail.
func TestSpawnAgentChatAgentTypeRootOnly(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-root-only",
	)

	ctx = withSubagentDelegatedKey(ctx, t, db, parentChat.OwnerID)
	childResp := runSpawnAgentTool(ctx, t, server, parentChat, spawnAgentArgs{
		Type:   subagentTypeGeneral,
		Prompt: "delegate work",
	})
	childID := requireSpawnAgentChildChatID(t, childResp)
	childChat, err := db.GetChatByID(ctx, childID)
	require.NoError(t, err)

	resp := runSpawnAgentTool(ctx, t, server, childChat, spawnAgentArgs{
		Type:   subagentTypeChatAgentPrefix + "coder",
		Prompt: "nested delegation",
	})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "delegated chats cannot create child subagents")
}

// TestBuildSpawnAgentDescriptionListsChatAgents verifies that the
// spawn_agent tool description enumerates builtin and custom agents.
func TestBuildSpawnAgentDescriptionListsChatAgents(t *testing.T) {
	t.Parallel()

	db, ps := dbtestutil.NewDB(t)
	server := newInternalTestServer(t, db, ps, chatprovider.ProviderAPIKeys{})

	ctx := chatdTestContext(t)
	user, org, model := seedInternalChatDeps(t, db)
	dbgen.ChatAgent(t, db, database.ChatAgent{
		OrganizationID: uuid.NullUUID{UUID: org.ID, Valid: true},
		Slug:           "catalog-agent",
		Name:           "Catalog Agent",
		Description:    "Reviews catalog entries.",
		CreatedBy:      user.ID,
	})
	parentChat := createInternalParentChat(
		ctx, t, server, db, org.ID, user.ID, model.ID, "parent-catalog",
	)

	description := buildSpawnAgentDescription(ctx, server, parentChat)
	require.Contains(t, description, subagentTypeChatAgentPrefix+"coder")
	require.Contains(t, description, subagentTypeChatAgentPrefix+"catalog-agent")
	require.Contains(t, description, "Reviews catalog entries.")
}
