package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// ChatPersona bundles a system prompt with a preferred model. Builtin
// personas are served from an in-memory catalog; deployment and
// organization personas are stored in the database. A nil
// OrganizationID means the persona is deployment-scoped.
type ChatPersona struct {
	ID             uuid.UUID  `json:"id" format:"uuid"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" format:"uuid"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Icon           string     `json:"icon"`
	SystemPrompt   string     `json:"system_prompt"`
	ModelConfigID  *uuid.UUID `json:"model_config_id,omitempty" format:"uuid"`
	Builtin        bool       `json:"builtin"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at" format:"date-time"`
	UpdatedAt      time.Time  `json:"updated_at" format:"date-time"`
}

// ChatAgent is a named invocable entry that points at a persona and
// optionally appends to its prompt or overrides its model. A nil
// OrganizationID means the agent is deployment-scoped.
type ChatAgent struct {
	ID             uuid.UUID  `json:"id" format:"uuid"`
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" format:"uuid"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Icon           string     `json:"icon"`
	PersonaID      uuid.UUID  `json:"persona_id" format:"uuid"`
	PromptAppend   string     `json:"prompt_append"`
	ModelConfigID  *uuid.UUID `json:"model_config_id,omitempty" format:"uuid"`
	Builtin        bool       `json:"builtin"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at" format:"date-time"`
	UpdatedAt      time.Time  `json:"updated_at" format:"date-time"`
}

// ChatAgentSummary identifies the chat agent a chat was created as,
// for display attribution. Builtin reports whether the agent is an
// in-memory builtin catalog entry rather than a database row.
type ChatAgentSummary struct {
	ID      uuid.UUID `json:"id" format:"uuid"`
	Slug    string    `json:"slug,omitempty"`
	Name    string    `json:"name,omitempty"`
	Icon    string    `json:"icon,omitempty"`
	Builtin bool      `json:"builtin,omitempty"`
}

// CreateChatPersonaRequest creates a chat persona. A nil or zero
// OrganizationID creates a deployment-scoped persona.
type CreateChatPersonaRequest struct {
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" format:"uuid"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	Description    string     `json:"description,omitempty"`
	Icon           string     `json:"icon,omitempty"`
	SystemPrompt   string     `json:"system_prompt"`
	ModelConfigID  *uuid.UUID `json:"model_config_id,omitempty" format:"uuid"`
	Enabled        *bool      `json:"enabled,omitempty"`
}

// UpdateChatPersonaRequest updates a chat persona. Nil fields are left
// unchanged. Setting ModelConfigID to the zero UUID clears the model
// preference. The slug and scope are immutable.
type UpdateChatPersonaRequest struct {
	Name          *string    `json:"name,omitempty"`
	Description   *string    `json:"description,omitempty"`
	Icon          *string    `json:"icon,omitempty"`
	SystemPrompt  *string    `json:"system_prompt,omitempty"`
	ModelConfigID *uuid.UUID `json:"model_config_id,omitempty" format:"uuid"`
	Enabled       *bool      `json:"enabled,omitempty"`
}

// CreateChatAgentRequest creates a chat agent. A nil or zero
// OrganizationID creates a deployment-scoped agent.
type CreateChatAgentRequest struct {
	OrganizationID *uuid.UUID `json:"organization_id,omitempty" format:"uuid"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	Description    string     `json:"description,omitempty"`
	Icon           string     `json:"icon,omitempty"`
	PersonaID      uuid.UUID  `json:"persona_id" format:"uuid"`
	PromptAppend   string     `json:"prompt_append,omitempty"`
	ModelConfigID  *uuid.UUID `json:"model_config_id,omitempty" format:"uuid"`
	Enabled        *bool      `json:"enabled,omitempty"`
}

// UpdateChatAgentRequest updates a chat agent. Nil fields are left
// unchanged. Setting ModelConfigID to the zero UUID clears the model
// override. The slug and scope are immutable.
type UpdateChatAgentRequest struct {
	Name          *string    `json:"name,omitempty"`
	Description   *string    `json:"description,omitempty"`
	Icon          *string    `json:"icon,omitempty"`
	PersonaID     *uuid.UUID `json:"persona_id,omitempty" format:"uuid"`
	PromptAppend  *string    `json:"prompt_append,omitempty"`
	ModelConfigID *uuid.UUID `json:"model_config_id,omitempty" format:"uuid"`
	Enabled       *bool      `json:"enabled,omitempty"`
}

// ChatPersonas lists builtin and deployment-scoped chat personas and,
// when organizationID is not uuid.Nil, that organization's personas.
func (c *ExperimentalClient) ChatPersonas(ctx context.Context, organizationID uuid.UUID) ([]ChatPersona, error) {
	endpoint := "/api/experimental/chats/personas"
	if organizationID != uuid.Nil {
		endpoint += "?organization=" + url.QueryEscape(organizationID.String())
	}
	res, err := c.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var personas []ChatPersona
	return personas, json.NewDecoder(res.Body).Decode(&personas)
}

// CreateChatPersona creates a chat persona.
func (c *ExperimentalClient) CreateChatPersona(ctx context.Context, req CreateChatPersonaRequest) (ChatPersona, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/chats/personas", req)
	if err != nil {
		return ChatPersona{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return ChatPersona{}, ReadBodyAsError(res)
	}

	var persona ChatPersona
	return persona, json.NewDecoder(res.Body).Decode(&persona)
}

// UpdateChatPersona updates a chat persona.
func (c *ExperimentalClient) UpdateChatPersona(ctx context.Context, personaID uuid.UUID, req UpdateChatPersonaRequest) (ChatPersona, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/chats/personas/%s", personaID), req)
	if err != nil {
		return ChatPersona{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatPersona{}, ReadBodyAsError(res)
	}

	var persona ChatPersona
	return persona, json.NewDecoder(res.Body).Decode(&persona)
}

// DeleteChatPersona soft-deletes a chat persona.
func (c *ExperimentalClient) DeleteChatPersona(ctx context.Context, personaID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/chats/personas/%s", personaID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// ChatAgents lists builtin and deployment-scoped chat agents and, when
// organizationID is not uuid.Nil, that organization's agents.
func (c *ExperimentalClient) ChatAgents(ctx context.Context, organizationID uuid.UUID) ([]ChatAgent, error) {
	endpoint := "/api/experimental/chats/agents"
	if organizationID != uuid.Nil {
		endpoint += "?organization=" + url.QueryEscape(organizationID.String())
	}
	res, err := c.Request(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var agents []ChatAgent
	return agents, json.NewDecoder(res.Body).Decode(&agents)
}

// CreateChatAgent creates a chat agent.
func (c *ExperimentalClient) CreateChatAgent(ctx context.Context, req CreateChatAgentRequest) (ChatAgent, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/chats/agents", req)
	if err != nil {
		return ChatAgent{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return ChatAgent{}, ReadBodyAsError(res)
	}

	var agent ChatAgent
	return agent, json.NewDecoder(res.Body).Decode(&agent)
}

// UpdateChatAgent updates a chat agent.
func (c *ExperimentalClient) UpdateChatAgent(ctx context.Context, agentID uuid.UUID, req UpdateChatAgentRequest) (ChatAgent, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/chats/agents/%s", agentID), req)
	if err != nil {
		return ChatAgent{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAgent{}, ReadBodyAsError(res)
	}

	var agent ChatAgent
	return agent, json.NewDecoder(res.Body).Decode(&agent)
}

// DeleteChatAgent soft-deletes a chat agent.
func (c *ExperimentalClient) DeleteChatAgent(ctx context.Context, agentID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/chats/agents/%s", agentID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
