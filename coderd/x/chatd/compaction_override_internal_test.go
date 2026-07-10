package chatd

import (
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

func TestResolveCompactionOverrideConfig_Unset(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return("", nil)

	server := titleOverrideTestServer(db, logger)
	_, overrideSet, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.False(t, overrideSet)
}

func TestResolveCompactionOverrideConfig_ReadDBError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return("", sql.ErrConnDone)

	server := titleOverrideTestServer(db, logger)
	_, overrideSet, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.Error(t, err)
	require.ErrorContains(t, err, "read compaction model override")
	require.False(t, overrideSet)
}

func TestResolveCompactionOverrideConfig_MalformedFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return("not-a-uuid", nil)

	server := titleOverrideTestServer(db, logger)
	_, overrideSet, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.False(t, overrideSet)
}

func TestResolveCompactionOverrideConfig_DeletedConfigFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	missingID := uuid.New()

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(missingID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), missingID).Return(database.ChatModelConfig{}, sql.ErrNoRows)

	server := titleOverrideTestServer(db, logger)
	_, overrideSet, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.False(t, overrideSet)
}

func TestResolveCompactionOverrideConfig_DisabledConfigFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", false)

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)

	server := titleOverrideTestServer(db, logger)
	_, overrideSet, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.False(t, overrideSet)
}

func TestResolveCompactionOverrideConfig_MissingCredentialsFallsBack(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(database.AIProvider{
		ID:      providerID,
		Type:    database.AIProviderTypeOpenai,
		Enabled: true,
	}, nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return(nil, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	_, overrideSet, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.False(t, overrideSet)
}

func TestCompactionOverride_SetUsable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	logger := slogtest.Make(t, &slogtest.Options{IgnoreErrors: true})
	chat, _ := titleOverrideTestChatAndMessages(t)
	overrideConfig := titleOverrideModelConfig("gpt-4.1", true)
	providerID := uuid.New()
	overrideConfig.AIProviderID = uuid.NullUUID{UUID: providerID, Valid: true}

	db.EXPECT().GetChatCompactionModelOverride(gomock.Any()).Return(overrideConfig.ID.String(), nil)
	db.EXPECT().GetChatModelConfigByID(gomock.Any(), overrideConfig.ID).Return(overrideConfig, nil)
	db.EXPECT().GetAIProviderByID(gomock.Any(), providerID).Return(aibridgeTestAIProvider(providerID, "primary-openai", database.AIProviderTypeOpenai), nil).AnyTimes()
	db.EXPECT().GetAIProviderKeysByProviderID(gomock.Any(), providerID).Return([]database.AIProviderKey{{
		ProviderID: providerID,
		APIKey:     "test-key",
	}}, nil).AnyTimes()

	server := titleOverrideTestServer(db, logger)
	resolvedConfig, overrideSet, err := server.resolveCompactionOverrideConfig(ctx, chat)
	require.NoError(t, err)
	require.True(t, overrideSet)
	require.Equal(t, overrideConfig.ID, resolvedConfig.ID)

	override, err := server.buildCompactionOverrideModel(
		ctx,
		chat,
		resolvedConfig,
		modelBuildOptions{ActiveAPIKeyID: uuid.NewString()},
	)
	require.NoError(t, err)
	require.NotNil(t, override.model)
	require.Equal(t, overrideConfig.ID, override.modelConfig.ID)
	require.Equal(t, "openai", override.resolvedProvider)
	require.Equal(t, "gpt-4.1", override.resolvedModel)
}
