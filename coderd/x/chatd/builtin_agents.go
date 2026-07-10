package chatd

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
)

// Builtin chat personas and agents are deployment-scoped database rows
// with fixed well-known IDs, seeded (and refreshed) at coderd startup
// from the in-repo catalog below. They are always enabled and can never
// be updated or deleted through the API. Deployment and organization
// entries created by admins live alongside them and are managed through
// the chat persona and agent APIs.
var (
	BuiltinChatPersonaSWEID              = uuid.MustParse("c0defade-0000-4000-8000-000000000001")
	BuiltinChatPersonaGeneralAssistantID = uuid.MustParse("c0defade-0000-4000-8000-000000000002")
	BuiltinChatPersonaCodeReviewerID     = uuid.MustParse("c0defade-0000-4000-8000-000000000003")

	BuiltinChatAgentCoderID     = uuid.MustParse("c0defade-0000-4000-8000-000000000101")
	BuiltinChatAgentAssistantID = uuid.MustParse("c0defade-0000-4000-8000-000000000102")
	BuiltinChatAgentReviewerID  = uuid.MustParse("c0defade-0000-4000-8000-000000000103")
)

const builtinGeneralAssistantSystemPrompt = `You are a helpful general-purpose assistant running inside the Coder product. Answer questions clearly and concisely, help with research, writing, and analysis, and use the tools available to you when they help complete the task. Prefer accuracy over speculation: verify claims with tools when possible and say so when you are unsure.`

const builtinCodeReviewerSystemPrompt = `You are a careful code reviewer running inside the Coder product. Review code changes for correctness, security issues, performance problems, and maintainability. Read the surrounding code before judging a change, point at specific lines when raising an issue, explain why each issue matters, and suggest concrete fixes. Distinguish blocking defects from optional style suggestions, and do not invent issues when a change is sound.`

// builtinChatPersonaSeeds returns the canonical builtin persona rows.
// SeedBuiltinChatCatalog upserts these at startup so prompt changes
// ship with new releases.
func builtinChatPersonaSeeds() []database.UpsertBuiltinChatPersonaParams {
	return []database.UpsertBuiltinChatPersonaParams{
		{
			ID:           BuiltinChatPersonaSWEID,
			Slug:         "swe",
			Name:         "Software Engineer",
			Description:  "The default software-engineering persona used by the Coder agent.",
			Icon:         "",
			SystemPrompt: DefaultSystemPrompt,
		},
		{
			ID:           BuiltinChatPersonaGeneralAssistantID,
			Slug:         "general-assistant",
			Name:         "General Assistant",
			Description:  "A general-purpose assistant for questions, research, and writing.",
			Icon:         "",
			SystemPrompt: builtinGeneralAssistantSystemPrompt,
		},
		{
			ID:           BuiltinChatPersonaCodeReviewerID,
			Slug:         "code-reviewer",
			Name:         "Code Reviewer",
			Description:  "Reviews code changes for correctness, security, and maintainability.",
			Icon:         "",
			SystemPrompt: builtinCodeReviewerSystemPrompt,
		},
	}
}

// builtinChatAgentSeeds returns the canonical builtin agent rows.
func builtinChatAgentSeeds() []database.UpsertBuiltinChatAgentParams {
	return []database.UpsertBuiltinChatAgentParams{
		{
			ID:           BuiltinChatAgentCoderID,
			Slug:         "coder",
			Name:         "Coder",
			Description:  "The default Coder software-engineering agent.",
			Icon:         "",
			PersonaID:    BuiltinChatPersonaSWEID,
			PromptAppend: "",
		},
		{
			ID:           BuiltinChatAgentAssistantID,
			Slug:         "assistant",
			Name:         "Assistant",
			Description:  "A general-purpose assistant.",
			Icon:         "",
			PersonaID:    BuiltinChatPersonaGeneralAssistantID,
			PromptAppend: "",
		},
		{
			ID:           BuiltinChatAgentReviewerID,
			Slug:         "reviewer",
			Name:         "Reviewer",
			Description:  "Reviews code changes.",
			Icon:         "",
			PersonaID:    BuiltinChatPersonaCodeReviewerID,
			PromptAppend: "",
		},
	}
}

// SeedBuiltinChatCatalog upserts the builtin personas and agents into
// the database, refreshing their canonical values. It runs at coderd
// startup and is idempotent. Personas seed before agents so the
// persona_id foreign key is always satisfied.
func SeedBuiltinChatCatalog(ctx context.Context, db database.Store) error {
	for _, persona := range builtinChatPersonaSeeds() {
		if err := db.UpsertBuiltinChatPersona(ctx, persona); err != nil {
			return xerrors.Errorf("seed builtin chat persona %q: %w", persona.Slug, err)
		}
	}
	for _, agent := range builtinChatAgentSeeds() {
		if err := db.UpsertBuiltinChatAgent(ctx, agent); err != nil {
			return xerrors.Errorf("seed builtin chat agent %q: %w", agent.Slug, err)
		}
	}
	return nil
}

// IsBuiltinChatPersonaSlug reports whether the slug is reserved by a
// builtin persona. Reservation applies across scopes so organization
// entries cannot shadow builtins.
func IsBuiltinChatPersonaSlug(slug string) bool {
	for _, persona := range builtinChatPersonaSeeds() {
		if persona.Slug == slug {
			return true
		}
	}
	return false
}

// IsBuiltinChatAgentSlug reports whether the slug is reserved by a
// builtin agent. Reservation applies across scopes so organization
// entries cannot shadow builtins.
func IsBuiltinChatAgentSlug(slug string) bool {
	for _, agent := range builtinChatAgentSeeds() {
		if agent.Slug == slug {
			return true
		}
	}
	return false
}
