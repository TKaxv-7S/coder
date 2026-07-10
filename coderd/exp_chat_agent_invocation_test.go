package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbgen"
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
			AgentID:        &agentID,
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
			CreatedBy:      firstUser.UserID,
		})
		agent := dbgen.ChatAgent(t, db, database.ChatAgent{
			OrganizationID: uuid.NullUUID{UUID: firstUser.OrganizationID, Valid: true},
			Slug:           "invoke-agent",
			Name:           "Invoke Agent",
			Icon:           "/emojis/1f916.png",
			PersonaID:      persona.ID,
			PromptAppend:   "Answer briefly.",
			CreatedBy:      firstUser.UserID,
		})

		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("hello agent"),
			AgentID:        &agent.ID,
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
			CreatedBy:      firstUser.UserID,
		})
		disabledAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:      "disabled-invoke-agent",
			CreatedBy: firstUser.UserID,
		}, func(params *database.InsertChatAgentParams) {
			params.Enabled = false
		})

		cases := []struct {
			name    string
			agentID uuid.UUID
		}{
			{name: "Unknown", agentID: uuid.New()},
			{name: "Disabled", agentID: disabledAgent.ID},
			{name: "ForeignOrg", agentID: foreignAgent.ID},
		}
		for _, tc := range cases {
			agentID := tc.agentID
			_, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
				OrganizationID: firstUser.OrganizationID,
				Content:        newChatInput("should fail"),
				AgentID:        &agentID,
			})
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, "case %s", tc.name)
			require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), "case %s", tc.name)
		}
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

		persona := dbgen.ChatPersona(t, db, database.ChatPersona{
			Slug:      "precedence-persona",
			CreatedBy: firstUser.UserID,
		})
		agent := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:          "precedence-agent",
			PersonaID:     persona.ID,
			ModelConfigID: uuid.NullUUID{UUID: overrideModel.ID, Valid: true},
			CreatedBy:     firstUser.UserID,
		})

		// The agent's model preference wins over the deployment
		// default.
		chat, err := client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("agent model"),
			AgentID:        &agent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, overrideModel.ID, chat.LastModelConfigID)

		// An explicit request model beats the agent preference.
		chat, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("explicit model"),
			AgentID:        &agent.ID,
			ModelConfigID:  &defaultModel.ID,
		})
		require.NoError(t, err)
		require.Equal(t, defaultModel.ID, chat.LastModelConfigID)

		// A disabled agent preference falls through to the default.
		_, err = client.UpdateChatModelConfig(ctx, overrideModel.ID, codersdk.UpdateChatModelConfigRequest{
			Enabled: boolPtr(false),
		})
		require.NoError(t, err)
		chat, err = client.CreateChat(ctx, codersdk.CreateChatRequest{
			OrganizationID: firstUser.OrganizationID,
			Content:        newChatInput("fallback model"),
			AgentID:        &agent.ID,
		})
		require.NoError(t, err)
		require.Equal(t, defaultModel.ID, chat.LastModelConfigID)
	})
}

func boolPtr(v bool) *bool {
	return &v
}
