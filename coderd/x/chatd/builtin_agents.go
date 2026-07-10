package chatd

import (
	"context"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

// Builtin chat personas and agents are in-memory catalog entries with
// fixed well-known IDs. They are not stored in the database, are always
// enabled, and can never be updated or deleted. Deployment and
// organization entries live in the database and are managed through the
// chat persona and agent APIs.
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

// BuiltinChatPersonas returns the builtin persona rows. A fresh slice
// of fresh values is returned on every call so callers can safely
// mutate the results.
func BuiltinChatPersonas() []database.ChatPersona {
	return []database.ChatPersona{
		{
			ID:           BuiltinChatPersonaSWEID,
			Slug:         "swe",
			Name:         "Software Engineer",
			Description:  "The default software-engineering persona used by the Coder agent.",
			SystemPrompt: DefaultSystemPrompt,
			Enabled:      true,
		},
		{
			ID:           BuiltinChatPersonaGeneralAssistantID,
			Slug:         "general-assistant",
			Name:         "General Assistant",
			Description:  "A general-purpose assistant for questions, research, and writing.",
			SystemPrompt: builtinGeneralAssistantSystemPrompt,
			Enabled:      true,
		},
		{
			ID:           BuiltinChatPersonaCodeReviewerID,
			Slug:         "code-reviewer",
			Name:         "Code Reviewer",
			Description:  "Reviews code changes for correctness, security, and maintainability.",
			SystemPrompt: builtinCodeReviewerSystemPrompt,
			Enabled:      true,
		},
	}
}

// BuiltinChatAgents returns the builtin agent rows. A fresh slice of
// fresh values is returned on every call so callers can safely mutate
// the results.
func BuiltinChatAgents() []database.ChatAgent {
	return []database.ChatAgent{
		{
			ID:          BuiltinChatAgentCoderID,
			Slug:        "coder",
			Name:        "Coder",
			Description: "The default Coder software-engineering agent.",
			PersonaID:   BuiltinChatPersonaSWEID,
			Enabled:     true,
		},
		{
			ID:          BuiltinChatAgentAssistantID,
			Slug:        "assistant",
			Name:        "Assistant",
			Description: "A general-purpose assistant.",
			PersonaID:   BuiltinChatPersonaGeneralAssistantID,
			Enabled:     true,
		},
		{
			ID:          BuiltinChatAgentReviewerID,
			Slug:        "reviewer",
			Name:        "Reviewer",
			Description: "Reviews code changes.",
			PersonaID:   BuiltinChatPersonaCodeReviewerID,
			Enabled:     true,
		},
	}
}

// BuiltinChatPersonaByID returns the builtin persona with the given ID.
func BuiltinChatPersonaByID(id uuid.UUID) (database.ChatPersona, bool) {
	for _, persona := range BuiltinChatPersonas() {
		if persona.ID == id {
			return persona, true
		}
	}
	return database.ChatPersona{}, false
}

// BuiltinChatAgentByID returns the builtin agent with the given ID.
func BuiltinChatAgentByID(id uuid.UUID) (database.ChatAgent, bool) {
	for _, agent := range BuiltinChatAgents() {
		if agent.ID == id {
			return agent, true
		}
	}
	return database.ChatAgent{}, false
}

// IsBuiltinChatPersonaSlug reports whether the slug is reserved by a
// builtin persona.
func IsBuiltinChatPersonaSlug(slug string) bool {
	for _, persona := range BuiltinChatPersonas() {
		if persona.Slug == slug {
			return true
		}
	}
	return false
}

// IsBuiltinChatAgentSlug reports whether the slug is reserved by a
// builtin agent.
func IsBuiltinChatAgentSlug(slug string) bool {
	for _, agent := range BuiltinChatAgents() {
		if agent.Slug == slug {
			return true
		}
	}
	return false
}

// ResolveChatPersona returns the persona with the given ID, checking
// builtins first and falling back to the database. The returned bool
// reports whether the persona is builtin.
func ResolveChatPersona(ctx context.Context, db database.Store, id uuid.UUID) (database.ChatPersona, bool, error) {
	if persona, ok := BuiltinChatPersonaByID(id); ok {
		return persona, true, nil
	}
	persona, err := db.GetChatPersonaByID(ctx, id)
	if err != nil {
		return database.ChatPersona{}, false, err
	}
	return persona, false, nil
}

// ResolveChatAgent returns the agent with the given ID, checking
// builtins first and falling back to the database. The returned bool
// reports whether the agent is builtin.
func ResolveChatAgent(ctx context.Context, db database.Store, id uuid.UUID) (database.ChatAgent, bool, error) {
	if agent, ok := BuiltinChatAgentByID(id); ok {
		return agent, true, nil
	}
	agent, err := db.GetChatAgentByID(ctx, id)
	if err != nil {
		return database.ChatAgent{}, false, err
	}
	return agent, false, nil
}
