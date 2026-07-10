package chatd

import (
	"context"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
)

const compactionOverrideContext = "compaction"

func readCompactionModelOverride(
	ctx context.Context,
	db database.Store,
) (string, error) {
	//nolint:gocritic // Chatd is internal, not a user, so this read uses AsChatd.
	chatdCtx := dbauthz.AsChatd(ctx)
	raw, err := db.GetChatCompactionModelOverride(chatdCtx)
	if err != nil {
		return "", xerrors.Errorf(
			"get chat compaction model override: %w",
			err,
		)
	}
	return raw, nil
}

// compactionModelOverride carries the resolved deployment-wide compaction
// model override: the model to run compaction summaries with plus the
// identity metadata debug runs and prompt sanitization need.
type compactionModelOverride struct {
	modelConfig      database.ChatModelConfig
	model            fantasy.LanguageModel
	resolvedProvider string
	resolvedModel    string
}

// resolveCompactionOverrideConfig resolves the stored deployment-wide
// compaction model override to its model config. Unset, malformed, stale
// (deleted or disabled config or provider), and credential-less overrides
// fall back to the chat model (overrideSet is false; the shared resolver
// logs the reason). This runs on every generation prepare because the
// override's context limit feeds the compaction trigger; the model client
// is built separately by buildCompactionOverrideModel only when compaction
// actually runs.
func (p *Server) resolveCompactionOverrideConfig(
	ctx context.Context,
	chat database.Chat,
) (database.ChatModelConfig, bool, error) {
	raw, err := readCompactionModelOverride(ctx, p.db)
	if err != nil {
		return database.ChatModelConfig{}, false, xerrors.Errorf(
			"read compaction model override: %w",
			err,
		)
	}

	modelConfig, overrideEffort, overrideSet, err := p.resolveConfiguredModelOverride(
		ctx,
		compactionOverrideContext,
		raw,
		chat.OwnerID,
		p.resolveModelConfigAndNormalizedProvider,
		func(ctx context.Context, ownerID uuid.UUID, aiProviderID uuid.UUID) (chatprovider.ProviderAPIKeys, error) {
			return p.resolveUserProviderAPIKeys(ctx, ownerID, aiProviderID)
		},
		modelOverrideFailureModeSoft,
	)
	if err != nil || !overrideSet {
		return database.ChatModelConfig{}, overrideSet, err
	}
	return withResolvedReasoningEffort(modelConfig, overrideEffort), true, nil
}

// buildCompactionOverrideModel resolves the route and constructs the model
// client for a usable compaction override config. Errors are hard failures:
// a configured, usable override that cannot be routed or constructed must
// fail the generation visibly instead of silently compacting with the chat
// model. Callers invoke this from the compact generation action, so a
// broken override cannot fail turns that finish without compacting.
func (p *Server) buildCompactionOverrideModel(
	ctx context.Context,
	chat database.Chat,
	modelConfig database.ChatModelConfig,
	modelOpts modelBuildOptions,
) (compactionModelOverride, error) {
	//nolint:gocritic // Compaction overrides need chatd-scoped provider reads for user-owned chats.
	route, err := p.resolveModelRouteForConfig(dbauthz.AsChatd(ctx), chat.OwnerID, modelConfig)
	if err != nil {
		return compactionModelOverride{}, xerrors.Errorf(
			"resolve compaction model override route: %w",
			err,
		)
	}
	resolvedProvider, resolvedModel, err := chatprovider.ResolveModelWithProviderHint(
		modelConfig.Model,
		route.ModelProviderHint,
	)
	if err != nil {
		return compactionModelOverride{}, xerrors.Errorf(
			"resolve compaction model override metadata: %w",
			err,
		)
	}
	model, _, err := p.newDebugAwareModel(ctx, modelClientRequest{
		Chat:         chat,
		ModelName:    modelConfig.Model,
		UserAgent:    chatprovider.UserAgent(),
		ExtraHeaders: chatprovider.CoderHeaders(chat),
	}, route, modelOpts)
	if err != nil {
		return compactionModelOverride{}, xerrors.Errorf(
			"create compaction model override: %w",
			err,
		)
	}
	return compactionModelOverride{
		modelConfig:      modelConfig,
		model:            model,
		resolvedProvider: resolvedProvider,
		resolvedModel:    resolvedModel,
	}, nil
}
