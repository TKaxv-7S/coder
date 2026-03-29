package coderd_test

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestExternalAuthProvidersCRUD(t *testing.T) {
	t.Parallel()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		provider, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID:   "test-github",
			Type:         "github",
			ClientID:     "client-id-123",
			ClientSecret: "client-secret-456",
			DisplayName:  "Test GitHub",
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			Scopes:       []string{"repo", "user"},
		})
		require.NoError(t, err)

		assert.NotEqual(t, uuid.Nil, provider.ID)
		assert.Equal(t, "test-github", provider.ProviderID)
		assert.Equal(t, "github", provider.Type)
		assert.Equal(t, "client-id-123", provider.ClientID)
		assert.True(t, provider.HasClientSecret)
		assert.Equal(t, "database", provider.Source)
		assert.Equal(t, "Test GitHub", provider.DisplayName)
		assert.Equal(t, "https://github.com/login/oauth/authorize", provider.AuthURL)
		assert.Equal(t, "https://github.com/login/oauth/access_token", provider.TokenURL)
		assert.Equal(t, []string{"repo", "user"}, provider.Scopes)
		assert.False(t, provider.CreatedAt.IsZero())
		assert.False(t, provider.UpdatedAt.IsZero())
	})

	t.Run("CreateInvalidProviderID", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		invalidIDs := []string{
			"has spaces",
			"has_underscores",
			"-leading-dash",
			"",
		}
		for _, id := range invalidIDs {
			_, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
				ProviderID: id,
				Type:       "github",
				ClientID:   "client-id",
			})
			require.Error(t, err, "expected error for provider ID %q", id)
			var sdkErr *codersdk.Error
			require.ErrorAs(t, err, &sdkErr, "provider ID %q", id)
			assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode(), "provider ID %q", id)
		}
	})

	t.Run("CreateDuplicateProviderID", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID: "duplicate-provider",
			Type:       "github",
			ClientID:   "client-id-1",
		})
		require.NoError(t, err)

		// Second create with same provider ID should fail.
		_, err = client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID: "duplicate-provider",
			Type:       "github",
			ClientID:   "client-id-2",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		// The handler wraps DB unique constraint as 500.
		assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
	})

	t.Run("CreateInvalidRegex", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID: "test-provider",
			Type:       "github",
			ClientID:   "client-id",
			Regex:      "[invalid",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusBadRequest, sdkErr.StatusCode())
	})

	t.Run("CreateNilSliceFields", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Scopes, ExtraTokenKeys, and CodeChallengeMethods are nil.
		provider, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID: "nil-slices",
			Type:       "github",
			ClientID:   "client-id",
		})
		require.NoError(t, err)
		// Handler normalizes nil slices to empty slices.
		assert.Empty(t, provider.Scopes)
		assert.Empty(t, provider.ExtraTokenKeys)
		assert.Empty(t, provider.CodeChallengeMethods)
	})

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID:  "get-test",
			Type:        "gitlab",
			ClientID:    "get-client-id",
			DisplayName: "Get Test",
		})
		require.NoError(t, err)

		fetched, err := client.ExternalAuthProvider(ctx, created.ID)
		require.NoError(t, err)

		assert.Equal(t, created.ID, fetched.ID)
		assert.Equal(t, created.ProviderID, fetched.ProviderID)
		assert.Equal(t, created.Type, fetched.Type)
		assert.Equal(t, created.ClientID, fetched.ClientID)
		assert.Equal(t, created.DisplayName, fetched.DisplayName)
		assert.Equal(t, created.Source, fetched.Source)
	})

	t.Run("GetNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		_, err := client.ExternalAuthProvider(ctx, uuid.New())
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("List", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		providerIDs := []string{"list-one", "list-two", "list-three"}
		for _, pid := range providerIDs {
			_, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
				ProviderID: pid,
				Type:       "github",
				ClientID:   "client-" + pid,
			})
			require.NoError(t, err)
		}

		providers, err := client.ExternalAuthProviders(ctx)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(providers), 3)

		found := make(map[string]bool)
		for _, p := range providers {
			found[p.ProviderID] = true
		}
		for _, pid := range providerIDs {
			assert.True(t, found[pid], "expected provider %q in list", pid)
		}
	})

	t.Run("Update", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID:   "update-test",
			Type:         "github",
			ClientID:     "original-client-id",
			ClientSecret: "original-secret",
			DisplayName:  "Original Name",
		})
		require.NoError(t, err)

		newSecret := "new-secret"
		updated, err := client.UpdateExternalAuthProvider(ctx, created.ID, codersdk.UpdateExternalAuthProviderRequest{
			Type:         "gitlab",
			ClientID:     "new-client-id",
			ClientSecret: &newSecret,
			DisplayName:  "Updated Name",
			Scopes:       []string{"read_api"},
		})
		require.NoError(t, err)

		assert.Equal(t, created.ID, updated.ID)
		// ProviderID is immutable.
		assert.Equal(t, "update-test", updated.ProviderID)
		assert.Equal(t, "gitlab", updated.Type)
		assert.Equal(t, "new-client-id", updated.ClientID)
		assert.Equal(t, "Updated Name", updated.DisplayName)
		assert.True(t, updated.HasClientSecret)
		assert.Equal(t, []string{"read_api"}, updated.Scopes)
	})

	t.Run("UpdatePreservesSecret", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Create with a secret.
		created, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID:   "preserve-secret",
			Type:         "github",
			ClientID:     "client-id",
			ClientSecret: "my-secret",
		})
		require.NoError(t, err)
		require.True(t, created.HasClientSecret)

		// Update without setting ClientSecret (nil) — secret should
		// be preserved.
		updated, err := client.UpdateExternalAuthProvider(ctx, created.ID, codersdk.UpdateExternalAuthProviderRequest{
			Type:     "github",
			ClientID: "client-id",
		})
		require.NoError(t, err)
		assert.True(t, updated.HasClientSecret)

		// Create without a secret.
		noSecret, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID: "no-secret",
			Type:       "github",
			ClientID:   "client-id-2",
		})
		require.NoError(t, err)
		require.False(t, noSecret.HasClientSecret)

		// Update without setting ClientSecret — should remain false.
		updatedNoSecret, err := client.UpdateExternalAuthProvider(ctx, noSecret.ID, codersdk.UpdateExternalAuthProviderRequest{
			Type:     "github",
			ClientID: "client-id-2",
		})
		require.NoError(t, err)
		assert.False(t, updatedNoSecret.HasClientSecret)
	})

	t.Run("UpdateEnvSourced", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Database: db,
			Pubsub:   pubsub,
		})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Insert an env-sourced provider directly in the DB.
		envCfg, err := db.InsertExternalAuthProviderConfig(ctx, database.InsertExternalAuthProviderConfigParams{
			ID:                   uuid.New(),
			CreatedAt:            dbtime.Now(),
			UpdatedAt:            dbtime.Now(),
			ProviderID:           "env-github",
			Type:                 "github",
			ClientID:             "env-client-id",
			Source:               "env",
			Scopes:               []string{},
			ExtraTokenKeys:       []string{},
			CodeChallengeMethods: []string{},
			ClientSecretKeyID:    sql.NullString{},
		})
		require.NoError(t, err)

		_, err = client.UpdateExternalAuthProvider(ctx, envCfg.ID, codersdk.UpdateExternalAuthProviderRequest{
			Type:     "github",
			ClientID: "new-client-id",
		})
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		created, err := client.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID: "delete-test",
			Type:       "github",
			ClientID:   "client-id",
		})
		require.NoError(t, err)

		err = client.DeleteExternalAuthProvider(ctx, created.ID)
		require.NoError(t, err)

		// Verify it's gone.
		_, err = client.ExternalAuthProvider(ctx, created.ID)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("DeleteEnvSourced", func(t *testing.T) {
		t.Parallel()
		db, pubsub := dbtestutil.NewDB(t)
		client := coderdtest.New(t, &coderdtest.Options{
			Database: db,
			Pubsub:   pubsub,
		})
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		envCfg, err := db.InsertExternalAuthProviderConfig(ctx, database.InsertExternalAuthProviderConfigParams{
			ID:                   uuid.New(),
			CreatedAt:            dbtime.Now(),
			UpdatedAt:            dbtime.Now(),
			ProviderID:           "env-delete",
			Type:                 "github",
			ClientID:             "env-client-id",
			Source:               "env",
			Scopes:               []string{},
			ExtraTokenKeys:       []string{},
			CodeChallengeMethods: []string{},
			ClientSecretKeyID:    sql.NullString{},
		})
		require.NoError(t, err)

		err = client.DeleteExternalAuthProvider(ctx, envCfg.ID)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusForbidden, sdkErr.StatusCode())
	})

	t.Run("DeleteNotFound", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		_ = coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		err := client.DeleteExternalAuthProvider(ctx, uuid.New())
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
	})

	t.Run("NonAdminForbidden", func(t *testing.T) {
		t.Parallel()
		client := coderdtest.New(t, nil)
		owner := coderdtest.CreateFirstUser(t, client)
		userClient, _ := coderdtest.CreateAnotherUser(t, client, owner.OrganizationID)
		ctx := testutil.Context(t, testutil.WaitLong)

		// Authorization is enforced by dbauthz on
		// ResourceDeploymentConfig. Non-owners get a wrapped
		// authorization error that surfaces as HTTP 500 from the
		// generic error handler.

		// List should fail for non-admin.
		_, err := userClient.ExternalAuthProviders(ctx)
		require.Error(t, err)
		var sdkErr *codersdk.Error
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())

		// Create should fail for non-admin.
		_, err = userClient.CreateExternalAuthProvider(ctx, codersdk.CreateExternalAuthProviderRequest{
			ProviderID: "forbidden-create",
			Type:       "github",
			ClientID:   "client-id",
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())

		// Get should fail for non-admin.
		_, err = userClient.ExternalAuthProvider(ctx, uuid.New())
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())

		// Update should fail for non-admin.
		_, err = userClient.UpdateExternalAuthProvider(ctx, uuid.New(), codersdk.UpdateExternalAuthProviderRequest{
			Type:     "github",
			ClientID: "client-id",
		})
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())

		// Delete should fail for non-admin.
		err = userClient.DeleteExternalAuthProvider(ctx, uuid.New())
		require.Error(t, err)
		require.ErrorAs(t, err, &sdkErr)
		assert.Equal(t, http.StatusInternalServerError, sdkErr.StatusCode())
	})
}
