package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/enterprise/coderd/license"
	"github.com/coder/coder/v2/testutil"
)

func chatAgentsTestClient(t *testing.T) (*codersdk.Client, codersdk.CreateFirstUserResponse) {
	t.Helper()
	return coderdenttest.New(t, &coderdenttest.Options{
		LicenseOptions: &coderdenttest.LicenseOptions{
			Features: license.Features{
				codersdk.FeatureChatAgents: 1,
			},
		},
	})
}

func TestChatPersonasCRUD(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := chatAgentsTestClient(t)
		exp := codersdk.NewExperimentalClient(client)

		created, err := exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			Slug:         "docs-writer",
			Name:         "Docs Writer",
			Description:  "Writes documentation.",
			SystemPrompt: "You write excellent documentation.",
		})
		require.NoError(t, err)
		require.Equal(t, "docs-writer", created.Slug)
		require.False(t, created.Builtin)
		require.True(t, created.Enabled)
		require.Nil(t, created.OrganizationID)

		newName := "Documentation Writer"
		disabled := false
		updated, err := exp.UpdateChatPersona(ctx, created.ID, codersdk.UpdateChatPersonaRequest{
			Name:    &newName,
			Enabled: &disabled,
		})
		require.NoError(t, err)
		require.Equal(t, newName, updated.Name)
		require.False(t, updated.Enabled)
		// Unspecified fields are left unchanged.
		require.Equal(t, created.SystemPrompt, updated.SystemPrompt)

		personas, err := exp.ChatPersonas(ctx, uuid.Nil)
		require.NoError(t, err)
		var found bool
		for _, persona := range personas {
			if persona.ID == created.ID {
				found = true
			}
		}
		require.True(t, found)

		err = exp.DeleteChatPersona(ctx, created.ID)
		require.NoError(t, err)

		personas, err = exp.ChatPersonas(ctx, uuid.Nil)
		require.NoError(t, err)
		for _, persona := range personas {
			require.NotEqual(t, created.ID, persona.ID, "deleted persona still listed")
		}
	})

	t.Run("OrgScoped", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, first := chatAgentsTestClient(t)
		orgAdminClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID, rbac.ScopedRoleOrgAdmin(first.OrganizationID))
		exp := codersdk.NewExperimentalClient(orgAdminClient)

		orgID := first.OrganizationID
		created, err := exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			OrganizationID: &orgID,
			Slug:           "org-persona",
			Name:           "Org Persona",
			SystemPrompt:   "You are the org persona.",
		})
		require.NoError(t, err)
		require.NotNil(t, created.OrganizationID)
		require.Equal(t, orgID, *created.OrganizationID)

		// Org admins cannot create deployment-scoped personas.
		_, err = exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			Slug:         "sneaky-deployment-persona",
			Name:         "Sneaky",
			SystemPrompt: "You should not exist.",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("MemberDenied", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, first := chatAgentsTestClient(t)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)
		exp := codersdk.NewExperimentalClient(memberClient)

		orgID := first.OrganizationID
		for _, req := range []codersdk.CreateChatPersonaRequest{
			{Slug: "member-persona", Name: "Member Persona", SystemPrompt: "prompt"},
			{OrganizationID: &orgID, Slug: "member-org-persona", Name: "Member Org Persona", SystemPrompt: "prompt"},
		} {
			_, err := exp.CreateChatPersona(ctx, req)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr)
			require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		}
	})

	t.Run("NotEntitled", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := coderdenttest.New(t, &coderdenttest.Options{
			LicenseOptions: &coderdenttest.LicenseOptions{
				Features: license.Features{},
			},
		})
		exp := codersdk.NewExperimentalClient(client)

		_, err := exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			Slug:         "unlicensed-persona",
			Name:         "Unlicensed",
			SystemPrompt: "prompt",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
		require.Contains(t, sdkErr.Message, "Premium feature")
	})

	t.Run("Validation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := chatAgentsTestClient(t)
		exp := codersdk.NewExperimentalClient(client)

		cases := []struct {
			name   string
			req    codersdk.CreateChatPersonaRequest
			status int
		}{
			{
				name:   "InvalidSlug",
				req:    codersdk.CreateChatPersonaRequest{Slug: "Bad Slug!", Name: "Bad", SystemPrompt: "prompt"},
				status: http.StatusBadRequest,
			},
			{
				name:   "MissingName",
				req:    codersdk.CreateChatPersonaRequest{Slug: "no-name", SystemPrompt: "prompt"},
				status: http.StatusBadRequest,
			},
			{
				name:   "MissingSystemPrompt",
				req:    codersdk.CreateChatPersonaRequest{Slug: "no-prompt", Name: "No Prompt"},
				status: http.StatusBadRequest,
			},
			{
				name: "UnknownModelConfig",
				req: codersdk.CreateChatPersonaRequest{
					Slug: "bad-model", Name: "Bad Model", SystemPrompt: "prompt",
					ModelConfigID: ptr.Ref(uuid.New()),
				},
				status: http.StatusBadRequest,
			},
			{
				name:   "BuiltinSlugCollision",
				req:    codersdk.CreateChatPersonaRequest{Slug: "swe", Name: "Fake SWE", SystemPrompt: "prompt"},
				status: http.StatusConflict,
			},
		}
		for _, tc := range cases {
			_, err := exp.CreateChatPersona(ctx, tc.req)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, "case %s", tc.name)
			require.Equal(t, tc.status, sdkErr.StatusCode(), "case %s", tc.name)
		}

		// Duplicate slug in the same scope conflicts.
		_, err := exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			Slug: "dupe", Name: "Dupe", SystemPrompt: "prompt",
		})
		require.NoError(t, err)
		_, err = exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			Slug: "dupe", Name: "Dupe Again", SystemPrompt: "prompt",
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	})

	t.Run("BuiltinImmutable", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := chatAgentsTestClient(t)
		exp := codersdk.NewExperimentalClient(client)

		name := "Hacked"
		_, err := exp.UpdateChatPersona(ctx, chatd.BuiltinChatPersonaSWEID, codersdk.UpdateChatPersonaRequest{Name: &name})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		err = exp.DeleteChatPersona(ctx, chatd.BuiltinChatPersonaSWEID)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})
}

func TestChatAgentsCRUD(t *testing.T) {
	t.Parallel()

	t.Run("HappyPath", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := chatAgentsTestClient(t)
		exp := codersdk.NewExperimentalClient(client)

		// Agents may reference builtin personas.
		created, err := exp.CreateChatAgent(ctx, codersdk.CreateChatAgentRequest{
			Slug:         "docs-agent",
			Name:         "Docs Agent",
			PersonaID:    chatd.BuiltinChatPersonaGeneralAssistantID,
			PromptAppend: "Focus on documentation tasks.",
		})
		require.NoError(t, err)
		require.False(t, created.Builtin)
		require.Equal(t, chatd.BuiltinChatPersonaGeneralAssistantID, created.PersonaID)
		require.Equal(t, "Focus on documentation tasks.", created.PromptAppend)

		// Repoint at a database persona and update the append.
		persona, err := exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			Slug: "docs-persona", Name: "Docs Persona", SystemPrompt: "You write docs.",
		})
		require.NoError(t, err)
		newAppend := "Focus on tutorials."
		updated, err := exp.UpdateChatAgent(ctx, created.ID, codersdk.UpdateChatAgentRequest{
			PersonaID:    &persona.ID,
			PromptAppend: &newAppend,
		})
		require.NoError(t, err)
		require.Equal(t, persona.ID, updated.PersonaID)
		require.Equal(t, newAppend, updated.PromptAppend)

		err = exp.DeleteChatAgent(ctx, created.ID)
		require.NoError(t, err)

		agents, err := exp.ChatAgents(ctx, uuid.Nil)
		require.NoError(t, err)
		for _, agent := range agents {
			require.NotEqual(t, created.ID, agent.ID, "deleted agent still listed")
		}
	})

	t.Run("PersonaValidation", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, first := chatAgentsTestClient(t)
		exp := codersdk.NewExperimentalClient(client)

		// Unknown persona.
		_, err := exp.CreateChatAgent(ctx, codersdk.CreateChatAgentRequest{
			Slug: "orphan-agent", Name: "Orphan", PersonaID: uuid.New(),
		})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

		// Disabled persona.
		disabledPersona, err := exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			Slug: "disabled-persona", Name: "Disabled", SystemPrompt: "prompt",
			Enabled: ptr.Ref(false),
		})
		require.NoError(t, err)
		_, err = exp.CreateChatAgent(ctx, codersdk.CreateChatAgentRequest{
			Slug: "disabled-agent", Name: "Disabled Agent", PersonaID: disabledPersona.ID,
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

		// A deployment agent cannot reference an org persona.
		orgID := first.OrganizationID
		orgPersona, err := exp.CreateChatPersona(ctx, codersdk.CreateChatPersonaRequest{
			OrganizationID: &orgID,
			Slug:           "org-only-persona",
			Name:           "Org Only",
			SystemPrompt:   "prompt",
		})
		require.NoError(t, err)
		_, err = exp.CreateChatAgent(ctx, codersdk.CreateChatAgentRequest{
			Slug: "cross-scope-agent", Name: "Cross Scope", PersonaID: orgPersona.ID,
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())

		// An org agent can reference its own org's persona.
		created, err := exp.CreateChatAgent(ctx, codersdk.CreateChatAgentRequest{
			OrganizationID: &orgID,
			Slug:           "org-agent",
			Name:           "Org Agent",
			PersonaID:      orgPersona.ID,
		})
		require.NoError(t, err)
		require.NotNil(t, created.OrganizationID)
	})

	t.Run("BuiltinImmutable", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		client, _ := chatAgentsTestClient(t)
		exp := codersdk.NewExperimentalClient(client)

		name := "Hacked"
		_, err := exp.UpdateChatAgent(ctx, chatd.BuiltinChatAgentCoderID, codersdk.UpdateChatAgentRequest{Name: &name})
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		err = exp.DeleteChatAgent(ctx, chatd.BuiltinChatAgentCoderID)
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusForbidden, sdkErr.StatusCode())

		_, err = exp.CreateChatAgent(ctx, codersdk.CreateChatAgentRequest{
			Slug: "coder", Name: "Fake Coder", PersonaID: chatd.BuiltinChatPersonaSWEID,
		})
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusConflict, sdkErr.StatusCode())
	})
}
