package coderd_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestCreateChatWithAgent(t *testing.T) {
	t.Parallel()

	newChatInput := func(text string) []codersdk.ChatInputPart {
		return []codersdk.ChatInputPart{{
			Type: codersdk.ChatInputPartTypeText,
			Text: text,
		}}
	}

	t.Run("BuiltinAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		agentID := chatd.BuiltinChatAgentReviewerID
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("review something"),
			ChatAgentID:    &agentID,
		})
		require.NoError(t, err)
		require.NotNil(t, chat.Agent)
		require.Equal(t, agentID, chat.Agent.ID)
		require.Equal(t, "reviewer", chat.Agent.Slug)
		require.Equal(t, "Reviewer", chat.Agent.Name)

		dbChat, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID) //nolint:gocritic // Direct row assertion in test.
		require.NoError(t, err)
		require.True(t, dbChat.ChatAgentID.Valid)
		require.Equal(t, agentID, dbChat.ChatAgentID.UUID)
	})

	t.Run("DatabaseAgent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		persona := dbgen.ChatPersona(t, db, database.ChatPersona{
			OrganizationID: uuid.NullUUID{UUID: firstUser.OrganizationID, Valid: true},
			Slug:           "invoke-persona",
			SystemPrompt:   "You are the invocation test persona.",
			CreatedBy:      uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		agent := dbgen.ChatAgent(t, db, database.ChatAgent{
			OrganizationID: uuid.NullUUID{UUID: firstUser.OrganizationID, Valid: true},
			Slug:           "invoke-agent",
			Name:           "Invoke Agent",
			Icon:           "/emojis/1f916.png",
			PersonaID:      persona.ID,
			PromptAppend:   "Answer briefly.",
			CreatedBy:      uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("hello agent"),
			ChatAgentID:    &agent.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, chat.Agent)
		require.Equal(t, agent.ID, chat.Agent.ID)
		require.Equal(t, agent.Slug, chat.Agent.Slug)
		require.Equal(t, agent.Name, chat.Agent.Name)
		require.Equal(t, agent.Icon, chat.Agent.Icon)

		dbChat, err := db.GetChatByID(dbauthz.AsSystemRestricted(ctx), chat.ID) //nolint:gocritic // Direct row assertion in test.
		require.NoError(t, err)
		require.True(t, dbChat.ChatAgentID.Valid)
		require.Equal(t, agent.ID, dbChat.ChatAgentID.UUID)

		// The single-chat GET also carries the enriched summary.
		fetched, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, fetched.Agent)
		require.Equal(t, agent.Slug, fetched.Agent.Slug)
	})

	t.Run("Rejections", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		otherOrg := dbgen.Organization(t, db, database.Organization{})
		foreignAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
			OrganizationID: uuid.NullUUID{UUID: otherOrg.ID, Valid: true},
			Slug:           "foreign-invoke-agent",
			CreatedBy:      uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		disabledAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:      "disabled-invoke-agent",
			CreatedBy: uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		}, func(params *database.InsertChatAgentParams) {
			params.Enabled = false
		})

		// Agents whose persona was disabled or deleted after the agent
		// was created must be rejected at invocation time.
		disabledPersona := dbgen.ChatPersona(t, db, database.ChatPersona{
			Slug:      "disabled-invoke-persona",
			CreatedBy: uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		}, func(params *database.InsertChatPersonaParams) {
			params.Enabled = false
		})
		agentWithDisabledPersona := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:      "disabled-persona-invoke-agent",
			PersonaID: disabledPersona.ID,
			CreatedBy: uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		deletedPersona := dbgen.ChatPersona(t, db, database.ChatPersona{
			Slug:      "deleted-invoke-persona",
			CreatedBy: uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		agentWithDeletedPersona := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:      "deleted-persona-invoke-agent",
			PersonaID: deletedPersona.ID,
			CreatedBy: uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		require.NoError(t, db.UpdateChatPersonaDeletedByID(ownerCtx(ctx), deletedPersona.ID))

		cases := []struct {
			name    string
			agentID uuid.UUID
			message string
		}{
			{name: "Unknown", agentID: uuid.New(), message: "Chat agent does not exist"},
			{name: "Disabled", agentID: disabledAgent.ID, message: "Chat agent is disabled"},
			{name: "ForeignOrg", agentID: foreignAgent.ID, message: "Chat agent belongs to a different organization"},
			{name: "PersonaDisabled", agentID: agentWithDisabledPersona.ID, message: "persona is disabled"},
			{name: "PersonaDeleted", agentID: agentWithDeletedPersona.ID, message: "persona does not exist"},
		}
		for _, tc := range cases {
			agentID := tc.agentID
			_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content:        newChatInput("should fail"),
				ChatAgentID:    &agentID,
			})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, "case %s", tc.name)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), "case %s", tc.name)
			require.Contains(t, sdkErr.Message, tc.message, "case %s", tc.name)
		}
	})

	t.Run("AttributionSurvivesAgentDeletion", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		_ = createChatModelConfig(t, client)

		agent := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:      "deleted-attribution-agent",
			Name:      "Deleted Attribution Agent",
			Icon:      "/emojis/1f916.png",
			CreatedBy: uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("hello"),
			ChatAgentID:    &agent.ID,
		})
		require.NoError(t, err)

		// Soft-delete the agent; existing chats must keep their
		// attribution because GetChatAgentsByIDs includes deleted rows.
		require.NoError(t, db.UpdateChatAgentDeletedByID(ownerCtx(ctx), agent.ID))

		fetched, err := client.GetChat(ctx, chat.ID)
		require.NoError(t, err)
		require.NotNil(t, fetched.Agent)
		require.Equal(t, agent.Slug, fetched.Agent.Slug)
		require.Equal(t, agent.Name, fetched.Agent.Name)
		require.False(t, fetched.Agent.Builtin)

		chats, err := client.ListChats(ctx, nil)
		require.NoError(t, err)
		require.NotEmpty(t, chats)
		var found bool
		for _, listed := range chats {
			if listed.ID != chat.ID {
				continue
			}
			found = true
			require.NotNil(t, listed.Agent)
			require.Equal(t, agent.Slug, listed.Agent.Slug)
		}
		require.True(t, found)
	})

	t.Run("ModelPrecedence", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, db := newChatClientWithDatabase(t)
		firstUser := coderdtest.CreateFirstUser(t, client.Client)
		defaultModel := createChatModelConfig(t, client)

		// A second, non-default model config on a fresh enabled
		// provider serves as the agent's model preference.
		overrideProvider, err := client.CreateAIProvider(ctx, codersdk.CreateAIProviderRequest{
			Type:    codersdk.AIProviderType(coderdtest.TestChatProviderOpenAICompat),
			Name:    "agent-override-" + uuid.NewString(),
			BaseURL: chattest.OpenAI(t),
			Enabled: true,
			APIKeys: []string{coderdtest.TestChatProviderAPIKey},
		})
		require.NoError(t, err)
		contextLimit := int64(4096)
		overrideModel, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			AIProviderID: &overrideProvider.ID,
			Model:        coderdtest.TestChatModelOpenAICompat,
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)
		personaModel, err := client.CreateChatModelConfig(ctx, codersdk.CreateChatModelConfigRequest{
			AIProviderID: &overrideProvider.ID,
			Model:        coderdtest.TestChatModelOpenAICompat,
			ContextLimit: &contextLimit,
		})
		require.NoError(t, err)

		persona := dbgen.ChatPersona(t, db, database.ChatPersona{
			Slug:          "precedence-persona",
			ModelConfigID: uuid.NullUUID{UUID: personaModel.ID, Valid: true},
			CreatedBy:     uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})
		agent := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:          "precedence-agent",
			PersonaID:     persona.ID,
			ModelConfigID: uuid.NullUUID{UUID: overrideModel.ID, Valid: true},
			CreatedBy:     uuid.NullUUID{UUID: firstUser.UserID, Valid: true},
		})

		// The agent's model preference wins over the persona's and the
		// deployment default.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("agent model"),
			ChatAgentID:    &agent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, overrideModel.ID, chat.LastModelConfigID)

		// An explicit request model beats the agent preference.
		chat, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("explicit model"),
			ChatAgentID:    &agent.ID,
			ModelConfigID:  &defaultModel.ID,
		})
		require.NoError(t, err)
		require.Equal(t, defaultModel.ID, chat.LastModelConfigID)

		// A disabled agent preference falls through to the persona's
		// preference, not straight to the default.
		_, err = client.UpdateChatModelConfig(ctx, overrideModel.ID, codersdk.UpdateChatModelConfigRequest{
			Enabled: ptr.Ref(false),
		})
		require.NoError(t, err)
		chat, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("persona model"),
			ChatAgentID:    &agent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, personaModel.ID, chat.LastModelConfigID)

		// With both preferences disabled the default applies.
		_, err = client.UpdateChatModelConfig(ctx, personaModel.ID, codersdk.UpdateChatModelConfigRequest{
			Enabled: ptr.Ref(false),
		})
		require.NoError(t, err)
		chat, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("fallback model"),
			ChatAgentID:    &agent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, defaultModel.ID, chat.LastModelConfigID)
	})
}

// ownerCtx returns a context authorized as a deployment owner for
// direct store mutations in tests.
func ownerCtx(ctx context.Context) context.Context {
	return dbauthz.As(ctx, rbac.Subject{
		ID:     "owner",
		Roles:  rbac.RoleIdentifiers{rbac.RoleOwner()},
		Groups: []string{},
		Scope:  rbac.ScopeAll,
	})
}
