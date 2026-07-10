package coderd_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestListChatPersonas(t *testing.T) {
	t.Parallel()

	t.Run("BuiltinsAlwaysPresent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		exp := codersdk.NewExperimentalClient(memberClient)

		personas, err := exp.ChatPersonas(ctx, uuid.Nil)
		require.NoError(t, err)

		bySlug := make(map[string]codersdk.ChatPersona)
		for _, persona := range personas {
			bySlug[persona.Slug] = persona
		}
		for _, slug := range []string{"swe", "general-assistant", "code-reviewer"} {
			persona, ok := bySlug[slug]
			require.True(t, ok, "builtin persona %q missing", slug)
			require.True(t, persona.Builtin)
			require.True(t, persona.Enabled)
			require.Nil(t, persona.OrganizationID)
			require.NotEmpty(t, persona.SystemPrompt)
		}
	})

	t.Run("OrganizationFiltering", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		exp := codersdk.NewExperimentalClient(memberClient)

		otherOrg := dbgen.Organization(t, db, database.Organization{})
		deploymentPersona := dbgen.ChatPersona(t, db, database.ChatPersona{
			Slug:      "deployment-persona",
			CreatedBy: owner.UserID,
		})
		orgPersona := dbgen.ChatPersona(t, db, database.ChatPersona{
			OrganizationID: uuid.NullUUID{UUID: owner.OrganizationID, Valid: true},
			Slug:           "org-persona",
			CreatedBy:      owner.UserID,
		})
		otherOrgPersona := dbgen.ChatPersona(t, db, database.ChatPersona{
			OrganizationID: uuid.NullUUID{UUID: otherOrg.ID, Valid: true},
			Slug:           "other-org-persona",
			CreatedBy:      owner.UserID,
		})

		// Without an organization filter, only builtins and
		// deployment-scoped personas are returned.
		personas, err := exp.ChatPersonas(ctx, uuid.Nil)
		require.NoError(t, err)
		ids := make(map[uuid.UUID]struct{})
		for _, persona := range personas {
			ids[persona.ID] = struct{}{}
		}
		require.Contains(t, ids, deploymentPersona.ID)
		require.NotContains(t, ids, orgPersona.ID)
		require.NotContains(t, ids, otherOrgPersona.ID)

		// With the member's organization, that org's personas are
		// included; other orgs' personas are not.
		personas, err = exp.ChatPersonas(ctx, owner.OrganizationID)
		require.NoError(t, err)
		ids = make(map[uuid.UUID]struct{})
		for _, persona := range personas {
			ids[persona.ID] = struct{}{}
		}
		require.Contains(t, ids, deploymentPersona.ID)
		require.Contains(t, ids, orgPersona.ID)
		require.NotContains(t, ids, otherOrgPersona.ID)
		require.Contains(t, ids, chatd.BuiltinChatPersonaSWEID)

		// A member cannot list another organization's personas even
		// though the RBAC read grant is site-wide; the list endpoint
		// enforces org membership.
		_, err = exp.ChatPersonas(ctx, otherOrg.ID)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())

		// The owner can list any organization's personas.
		ownerExp := codersdk.NewExperimentalClient(client)
		personas, err = ownerExp.ChatPersonas(ctx, otherOrg.ID)
		require.NoError(t, err)
		ids = make(map[uuid.UUID]struct{})
		for _, persona := range personas {
			ids[persona.ID] = struct{}{}
		}
		require.Contains(t, ids, otherOrgPersona.ID)
	})
}

func TestListChatAgents(t *testing.T) {
	t.Parallel()

	t.Run("BuiltinsAlwaysPresent", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		exp := codersdk.NewExperimentalClient(memberClient)

		agents, err := exp.ChatAgents(ctx, uuid.Nil)
		require.NoError(t, err)

		bySlug := make(map[string]codersdk.ChatAgent)
		for _, agent := range agents {
			bySlug[agent.Slug] = agent
		}
		coder, ok := bySlug["coder"]
		require.True(t, ok, "builtin agent coder missing")
		require.True(t, coder.Builtin)
		require.Equal(t, chatd.BuiltinChatPersonaSWEID, coder.PersonaID)
		require.Contains(t, bySlug, "assistant")
		require.Contains(t, bySlug, "reviewer")
	})

	t.Run("OrganizationFiltering", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitLong)
		db, ps := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{Database: db, Pubsub: ps})
		owner := coderdtest.CreateFirstUser(t, client)
		memberClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		exp := codersdk.NewExperimentalClient(memberClient)

		otherOrg := dbgen.Organization(t, db, database.Organization{})
		deploymentAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
			Slug:      "deployment-agent",
			CreatedBy: owner.UserID,
		})
		orgAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
			OrganizationID: uuid.NullUUID{UUID: owner.OrganizationID, Valid: true},
			Slug:           "org-agent",
			CreatedBy:      owner.UserID,
		})
		otherOrgAgent := dbgen.ChatAgent(t, db, database.ChatAgent{
			OrganizationID: uuid.NullUUID{UUID: otherOrg.ID, Valid: true},
			Slug:           "other-org-agent",
			CreatedBy:      owner.UserID,
		})

		agents, err := exp.ChatAgents(ctx, uuid.Nil)
		require.NoError(t, err)
		ids := make(map[uuid.UUID]struct{})
		for _, agent := range agents {
			ids[agent.ID] = struct{}{}
		}
		require.Contains(t, ids, deploymentAgent.ID)
		require.NotContains(t, ids, orgAgent.ID)
		require.NotContains(t, ids, otherOrgAgent.ID)

		agents, err = exp.ChatAgents(ctx, owner.OrganizationID)
		require.NoError(t, err)
		ids = make(map[uuid.UUID]struct{})
		for _, agent := range agents {
			ids[agent.ID] = struct{}{}
		}
		require.Contains(t, ids, deploymentAgent.ID)
		require.Contains(t, ids, orgAgent.ID)
		require.NotContains(t, ids, otherOrgAgent.ID)
	})
}
