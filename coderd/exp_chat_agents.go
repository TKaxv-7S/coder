package coderd

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/audit"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd"
	"github.com/coder/coder/v2/codersdk"
)

// chatCatalogSlugRegex validates persona and agent slugs: lowercase
// alphanumeric segments separated by single hyphens.
var chatCatalogSlugRegex = regexp.MustCompile(`^[a-z0-9](-?[a-z0-9])*$`)

// Length limits for persona and agent text fields. Names and
// descriptions are embedded in the spawn_agent tool schema sent on
// every generation, and prompts are copied into every chat created
// with the persona or agent, so unbounded values would inflate every
// request to the model.
const (
	chatCatalogSlugMaxLen        = 64
	chatCatalogNameMaxLen        = 64
	chatCatalogDescriptionMaxLen = 512
	chatCatalogIconMaxLen        = 256
	chatCatalogPromptMaxLen      = 32768
)

func convertChatPersona(persona database.ChatPersona) codersdk.ChatPersona {
	out := codersdk.ChatPersona{
		ID:           persona.ID,
		Slug:         persona.Slug,
		Name:         persona.Name,
		Description:  persona.Description,
		Icon:         persona.Icon,
		SystemPrompt: persona.SystemPrompt,
		Builtin:      persona.Builtin,
		Enabled:      persona.Enabled,
		CreatedAt:    persona.CreatedAt,
		UpdatedAt:    persona.UpdatedAt,
	}
	if persona.OrganizationID.Valid {
		out.OrganizationID = ptr.Ref(persona.OrganizationID.UUID)
	}
	if persona.ModelConfigID.Valid {
		out.ModelConfigID = ptr.Ref(persona.ModelConfigID.UUID)
	}
	return out
}

func convertChatAgent(agent database.ChatAgent) codersdk.ChatAgent {
	out := codersdk.ChatAgent{
		ID:           agent.ID,
		Slug:         agent.Slug,
		Name:         agent.Name,
		Description:  agent.Description,
		Icon:         agent.Icon,
		PersonaID:    agent.PersonaID,
		PromptAppend: agent.PromptAppend,
		Builtin:      agent.Builtin,
		Enabled:      agent.Enabled,
		CreatedAt:    agent.CreatedAt,
		UpdatedAt:    agent.UpdatedAt,
	}
	if agent.OrganizationID.Valid {
		out.OrganizationID = ptr.Ref(agent.OrganizationID.UUID)
	}
	if agent.ModelConfigID.Valid {
		out.ModelConfigID = ptr.Ref(agent.ModelConfigID.UUID)
	}
	return out
}

// parseChatCatalogListOrganization parses the optional `organization`
// query parameter for the persona and agent list endpoints and, when
// present, verifies the caller can read that organization. Org-scoped
// personas and agents are member-readable through a site-wide RBAC
// grant that exists for deployment-scoped rows, so org membership is
// enforced here to keep one org's entries invisible to other orgs.
func (api *API) parseChatCatalogListOrganization(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := r.URL.Query().Get("organization")
	if raw == "" {
		return uuid.Nil, true
	}
	orgID, err := uuid.Parse(raw)
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid organization ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	if !api.Authorize(r, policy.ActionRead, rbac.ResourceOrganization.InOrg(orgID)) {
		httpapi.ResourceNotFound(rw)
		return uuid.Nil, false
	}
	return orgID, true
}

// validateChatCatalogSlug validates a new persona or agent slug and
// writes an error response when invalid.
func validateChatCatalogSlug(rw http.ResponseWriter, r *http.Request, slug string) bool {
	if slug == "" {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Slug is required.",
		})
		return false
	}
	if len(slug) > chatCatalogSlugMaxLen {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("Slug must be at most %d characters.", chatCatalogSlugMaxLen),
		})
		return false
	}
	if !chatCatalogSlugRegex.MatchString(slug) {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid slug.",
			Detail:  "Slugs must be lowercase alphanumeric segments separated by single hyphens.",
		})
		return false
	}
	return true
}

// validateChatCatalogFieldLen enforces a maximum length on a persona
// or agent text field, writing an error response when exceeded.
func validateChatCatalogFieldLen(rw http.ResponseWriter, r *http.Request, field, value string, maxLen int) bool {
	if len(value) > maxLen {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: fmt.Sprintf("%s must be at most %d characters.", field, maxLen),
		})
		return false
	}
	return true
}

// validateChatCatalogModelConfigID checks that a referenced chat model
// config exists and is not deleted. It writes an error response and
// returns false when the reference is invalid.
func (api *API) validateChatCatalogModelConfigID(rw http.ResponseWriter, r *http.Request, modelConfigID uuid.UUID) bool {
	ctx := r.Context()
	//nolint:gocritic // The caller already authorized the persona/agent write; model configs are deployment-wide.
	_, err := api.Database.GetChatModelConfigByID(dbauthz.AsChatd(ctx), modelConfigID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Chat model config does not exist.",
			})
			return false
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat model config.",
			Detail:  err.Error(),
		})
		return false
	}
	return true
}

// validateChatAgentPersona checks that the referenced persona exists,
// is enabled, and is visible in the agent's scope: builtin and
// deployment personas are usable everywhere, organization personas
// only by agents in the same organization. It writes an error response
// and returns false when the reference is invalid.
func (api *API) validateChatAgentPersona(rw http.ResponseWriter, r *http.Request, personaID uuid.UUID, agentOrgID uuid.NullUUID) bool {
	ctx := r.Context()
	persona, err := api.Database.GetChatPersonaByID(ctx, personaID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Chat persona does not exist.",
			})
			return false
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat persona.",
			Detail:  err.Error(),
		})
		return false
	}
	if !persona.Enabled {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat persona is disabled.",
		})
		return false
	}
	if persona.OrganizationID.Valid && (!agentOrgID.Valid || persona.OrganizationID.UUID != agentOrgID.UUID) {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Chat persona belongs to a different organization.",
			Detail:  "Agents may reference builtin personas, deployment personas, or personas in the agent's own organization.",
		})
		return false
	}
	return true
}

// @Summary List chat personas
// @ID list-chat-personas
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization query string false "Organization ID to include organization-scoped personas" format(uuid)
// @Success 200 {array} codersdk.ChatPersona
// @Router /api/experimental/chats/personas [get]
// @Description Experimental: this endpoint is subject to change.
func (api *API) listChatPersonas(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, ok := api.parseChatCatalogListOrganization(rw, r)
	if !ok {
		return
	}

	rows, err := api.Database.GetChatPersonas(ctx, orgID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat personas.",
			Detail:  err.Error(),
		})
		return
	}

	// Builtin rows are part of the result set: they are seeded as
	// deployment-scoped rows at startup.
	resp := make([]codersdk.ChatPersona, 0, len(rows))
	for _, persona := range rows {
		resp = append(resp, convertChatPersona(persona))
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// @Summary List chat agents
// @ID list-chat-agents
// @Security CoderSessionToken
// @Tags Chats
// @Produce json
// @Param organization query string false "Organization ID to include organization-scoped agents" format(uuid)
// @Success 200 {array} codersdk.ChatAgent
// @Router /api/experimental/chats/agents [get]
// @Description Experimental: this endpoint is subject to change.
func (api *API) listChatAgents(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, ok := api.parseChatCatalogListOrganization(rw, r)
	if !ok {
		return
	}

	rows, err := api.Database.GetChatAgents(ctx, orgID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to list chat agents.",
			Detail:  err.Error(),
		})
		return
	}

	// Builtin rows are part of the result set: they are seeded as
	// deployment-scoped rows at startup.
	resp := make([]codersdk.ChatAgent, 0, len(rows))
	for _, agent := range rows {
		resp = append(resp, convertChatAgent(agent))
	}
	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

// CreateChatPersona is registered by enterprise coderd behind a
// premium feature gate.
//
// @Summary Create chat persona
// @ID create-chat-persona
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.CreateChatPersonaRequest true "Create chat persona request"
// @Success 201 {object} codersdk.ChatPersona
// @Router /api/experimental/chats/personas [post]
// @Description Experimental: this endpoint is subject to change.
func (api *API) CreateChatPersona(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateChatPersonaRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	orgID := uuid.NullUUID{}
	if req.OrganizationID != nil && *req.OrganizationID != uuid.Nil {
		orgID = uuid.NullUUID{UUID: *req.OrganizationID, Valid: true}
	}

	aReq, commitAudit := audit.InitRequest[database.ChatPersona](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: orgID.UUID,
	})
	defer commitAudit()

	rbacObject := rbac.ResourceChatPersona
	if orgID.Valid {
		rbacObject = rbacObject.InOrg(orgID.UUID)
	}
	if !api.Authorize(r, policy.ActionCreate, rbacObject) {
		httpapi.Forbidden(rw)
		return
	}

	slug := strings.TrimSpace(req.Slug)
	if !validateChatCatalogSlug(rw, r, slug) {
		return
	}
	if chatd.IsBuiltinChatPersonaSlug(slug) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Slug is reserved by a builtin persona.",
		})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Name is required.",
		})
		return
	}
	if strings.TrimSpace(req.SystemPrompt) == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "System prompt is required.",
		})
		return
	}
	if !validateChatCatalogFieldLen(rw, r, "Name", name, chatCatalogNameMaxLen) ||
		!validateChatCatalogFieldLen(rw, r, "Description", req.Description, chatCatalogDescriptionMaxLen) ||
		!validateChatCatalogFieldLen(rw, r, "Icon", req.Icon, chatCatalogIconMaxLen) ||
		!validateChatCatalogFieldLen(rw, r, "System prompt", req.SystemPrompt, chatCatalogPromptMaxLen) {
		return
	}
	modelConfigID := uuid.NullUUID{}
	if req.ModelConfigID != nil && *req.ModelConfigID != uuid.Nil {
		if !api.validateChatCatalogModelConfigID(rw, r, *req.ModelConfigID) {
			return
		}
		modelConfigID = uuid.NullUUID{UUID: *req.ModelConfigID, Valid: true}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	persona, err := api.Database.InsertChatPersona(ctx, database.InsertChatPersonaParams{
		OrganizationID: orgID,
		Slug:           slug,
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		Icon:           strings.TrimSpace(req.Icon),
		SystemPrompt:   req.SystemPrompt,
		ModelConfigID:  modelConfigID,
		Enabled:        enabled,
		CreatedBy:      uuid.NullUUID{UUID: apiKey.UserID, Valid: true},
	})
	if err != nil {
		switch {
		case dbauthz.IsNotAuthorizedError(err):
			httpapi.Forbidden(rw)
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "A chat persona with this slug already exists in this scope.",
			})
		case database.IsForeignKeyViolation(err):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Organization or model config does not exist.",
				Detail:  err.Error(),
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create chat persona.",
				Detail:  err.Error(),
			})
		}
		return
	}
	aReq.New = persona

	httpapi.Write(ctx, rw, http.StatusCreated, convertChatPersona(persona))
}

// UpdateChatPersona is registered by enterprise coderd behind a
// premium feature gate.
//
// @Summary Update chat persona
// @ID update-chat-persona
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param persona path string true "Chat persona ID" format(uuid)
// @Param request body codersdk.UpdateChatPersonaRequest true "Update chat persona request"
// @Success 200 {object} codersdk.ChatPersona
// @Router /api/experimental/chats/personas/{persona} [patch]
// @Description Experimental: this endpoint is subject to change.
func (api *API) UpdateChatPersona(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	personaID, ok := parseChatPersonaID(rw, r)
	if !ok {
		return
	}

	var req codersdk.UpdateChatPersonaRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	existing, err := api.Database.GetChatPersonaByID(ctx, personaID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat persona.",
			Detail:  err.Error(),
		})
		return
	}
	if existing.Builtin {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Builtin personas cannot be modified.",
		})
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatPersona](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: existing.OrganizationID.UUID,
	})
	defer commitAudit()
	aReq.Old = existing

	params := database.UpdateChatPersonaParams{
		ID:            existing.ID,
		Name:          existing.Name,
		Description:   existing.Description,
		Icon:          existing.Icon,
		SystemPrompt:  existing.SystemPrompt,
		ModelConfigID: existing.ModelConfigID,
		Enabled:       existing.Enabled,
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Name cannot be empty.",
			})
			return
		}
		if !validateChatCatalogFieldLen(rw, r, "Name", name, chatCatalogNameMaxLen) {
			return
		}
		params.Name = name
	}
	if req.Description != nil {
		if !validateChatCatalogFieldLen(rw, r, "Description", *req.Description, chatCatalogDescriptionMaxLen) {
			return
		}
		params.Description = strings.TrimSpace(*req.Description)
	}
	if req.Icon != nil {
		if !validateChatCatalogFieldLen(rw, r, "Icon", *req.Icon, chatCatalogIconMaxLen) {
			return
		}
		params.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.SystemPrompt != nil {
		if strings.TrimSpace(*req.SystemPrompt) == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "System prompt cannot be empty.",
			})
			return
		}
		if !validateChatCatalogFieldLen(rw, r, "System prompt", *req.SystemPrompt, chatCatalogPromptMaxLen) {
			return
		}
		params.SystemPrompt = *req.SystemPrompt
	}
	if req.ModelConfigID != nil {
		if *req.ModelConfigID == uuid.Nil {
			params.ModelConfigID = uuid.NullUUID{}
		} else {
			if !api.validateChatCatalogModelConfigID(rw, r, *req.ModelConfigID) {
				return
			}
			params.ModelConfigID = uuid.NullUUID{UUID: *req.ModelConfigID, Valid: true}
		}
	}
	if req.Enabled != nil {
		params.Enabled = *req.Enabled
	}

	updated, err := api.Database.UpdateChatPersona(ctx, params)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat persona.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = updated

	httpapi.Write(ctx, rw, http.StatusOK, convertChatPersona(updated))
}

// DeleteChatPersona is registered by enterprise coderd behind a
// premium feature gate.
//
// @Summary Delete chat persona
// @ID delete-chat-persona
// @Security CoderSessionToken
// @Tags Chats
// @Param persona path string true "Chat persona ID" format(uuid)
// @Success 204
// @Router /api/experimental/chats/personas/{persona} [delete]
// @Description Experimental: this endpoint is subject to change.
func (api *API) DeleteChatPersona(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	personaID, ok := parseChatPersonaID(rw, r)
	if !ok {
		return
	}

	existing, err := api.Database.GetChatPersonaByID(ctx, personaID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat persona.",
			Detail:  err.Error(),
		})
		return
	}
	if existing.Builtin {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Builtin personas cannot be deleted.",
		})
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatPersona](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: existing.OrganizationID.UUID,
	})
	defer commitAudit()
	aReq.Old = existing

	// Personas are soft-deleted, so the foreign key from chat_agents
	// alone cannot protect enabled agents from losing their persona;
	// deletion is blocked here while non-deleted agents reference it.
	referencing, err := api.Database.CountChatAgentsByPersonaID(ctx, personaID)
	if err != nil {
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to check chat agents referencing the persona.",
			Detail:  err.Error(),
		})
		return
	}
	if referencing > 0 {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: fmt.Sprintf("Chat persona is referenced by %d chat agent(s).", referencing),
			Detail:  "Delete or repoint the referencing chat agents first.",
		})
		return
	}

	err = api.Database.UpdateChatPersonaDeletedByID(ctx, personaID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat persona.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

// CreateChatAgent is registered by enterprise coderd behind a premium
// feature gate.
//
// @Summary Create chat agent
// @ID create-chat-agent
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param request body codersdk.CreateChatAgentRequest true "Create chat agent request"
// @Success 201 {object} codersdk.ChatAgent
// @Router /api/experimental/chats/agents [post]
// @Description Experimental: this endpoint is subject to change.
func (api *API) CreateChatAgent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	apiKey := httpmw.APIKey(r)

	var req codersdk.CreateChatAgentRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	orgID := uuid.NullUUID{}
	if req.OrganizationID != nil && *req.OrganizationID != uuid.Nil {
		orgID = uuid.NullUUID{UUID: *req.OrganizationID, Valid: true}
	}

	aReq, commitAudit := audit.InitRequest[database.ChatAgent](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionCreate,
		OrganizationID: orgID.UUID,
	})
	defer commitAudit()

	rbacObject := rbac.ResourceChatAgent
	if orgID.Valid {
		rbacObject = rbacObject.InOrg(orgID.UUID)
	}
	if !api.Authorize(r, policy.ActionCreate, rbacObject) {
		httpapi.Forbidden(rw)
		return
	}

	slug := strings.TrimSpace(req.Slug)
	if !validateChatCatalogSlug(rw, r, slug) {
		return
	}
	if chatd.IsBuiltinChatAgentSlug(slug) {
		httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
			Message: "Slug is reserved by a builtin agent.",
		})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Name is required.",
		})
		return
	}
	if !validateChatCatalogFieldLen(rw, r, "Name", name, chatCatalogNameMaxLen) ||
		!validateChatCatalogFieldLen(rw, r, "Description", req.Description, chatCatalogDescriptionMaxLen) ||
		!validateChatCatalogFieldLen(rw, r, "Icon", req.Icon, chatCatalogIconMaxLen) ||
		!validateChatCatalogFieldLen(rw, r, "Prompt append", req.PromptAppend, chatCatalogPromptMaxLen) {
		return
	}
	if req.PersonaID == uuid.Nil {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message: "Persona ID is required.",
		})
		return
	}
	if !api.validateChatAgentPersona(rw, r, req.PersonaID, orgID) {
		return
	}
	modelConfigID := uuid.NullUUID{}
	if req.ModelConfigID != nil && *req.ModelConfigID != uuid.Nil {
		if !api.validateChatCatalogModelConfigID(rw, r, *req.ModelConfigID) {
			return
		}
		modelConfigID = uuid.NullUUID{UUID: *req.ModelConfigID, Valid: true}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	agent, err := api.Database.InsertChatAgent(ctx, database.InsertChatAgentParams{
		OrganizationID: orgID,
		Slug:           slug,
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		Icon:           strings.TrimSpace(req.Icon),
		PersonaID:      req.PersonaID,
		PromptAppend:   req.PromptAppend,
		ModelConfigID:  modelConfigID,
		Enabled:        enabled,
		CreatedBy:      uuid.NullUUID{UUID: apiKey.UserID, Valid: true},
	})
	if err != nil {
		switch {
		case dbauthz.IsNotAuthorizedError(err):
			httpapi.Forbidden(rw)
		case database.IsUniqueViolation(err):
			httpapi.Write(ctx, rw, http.StatusConflict, codersdk.Response{
				Message: "A chat agent with this slug already exists in this scope.",
			})
		case database.IsForeignKeyViolation(err):
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Organization, persona, or model config does not exist.",
				Detail:  err.Error(),
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: "Failed to create chat agent.",
				Detail:  err.Error(),
			})
		}
		return
	}
	aReq.New = agent

	httpapi.Write(ctx, rw, http.StatusCreated, convertChatAgent(agent))
}

// UpdateChatAgent is registered by enterprise coderd behind a premium
// feature gate.
//
// @Summary Update chat agent
// @ID update-chat-agent
// @Security CoderSessionToken
// @Tags Chats
// @Accept json
// @Produce json
// @Param agent path string true "Chat agent ID" format(uuid)
// @Param request body codersdk.UpdateChatAgentRequest true "Update chat agent request"
// @Success 200 {object} codersdk.ChatAgent
// @Router /api/experimental/chats/agents/{agent} [patch]
// @Description Experimental: this endpoint is subject to change.
func (api *API) UpdateChatAgent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agentID, ok := parseChatAgentID(rw, r)
	if !ok {
		return
	}

	var req codersdk.UpdateChatAgentRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	existing, err := api.Database.GetChatAgentByID(ctx, agentID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat agent.",
			Detail:  err.Error(),
		})
		return
	}
	if existing.Builtin {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Builtin agents cannot be modified.",
		})
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatAgent](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionWrite,
		OrganizationID: existing.OrganizationID.UUID,
	})
	defer commitAudit()
	aReq.Old = existing

	params := database.UpdateChatAgentParams{
		ID:            existing.ID,
		Name:          existing.Name,
		Description:   existing.Description,
		Icon:          existing.Icon,
		PersonaID:     existing.PersonaID,
		PromptAppend:  existing.PromptAppend,
		ModelConfigID: existing.ModelConfigID,
		Enabled:       existing.Enabled,
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Name cannot be empty.",
			})
			return
		}
		if !validateChatCatalogFieldLen(rw, r, "Name", name, chatCatalogNameMaxLen) {
			return
		}
		params.Name = name
	}
	if req.Description != nil {
		if !validateChatCatalogFieldLen(rw, r, "Description", *req.Description, chatCatalogDescriptionMaxLen) {
			return
		}
		params.Description = strings.TrimSpace(*req.Description)
	}
	if req.Icon != nil {
		if !validateChatCatalogFieldLen(rw, r, "Icon", *req.Icon, chatCatalogIconMaxLen) {
			return
		}
		params.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.PersonaID != nil {
		if *req.PersonaID == uuid.Nil {
			httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
				Message: "Persona ID cannot be empty.",
			})
			return
		}
		if !api.validateChatAgentPersona(rw, r, *req.PersonaID, existing.OrganizationID) {
			return
		}
		params.PersonaID = *req.PersonaID
	}
	if req.PromptAppend != nil {
		if !validateChatCatalogFieldLen(rw, r, "Prompt append", *req.PromptAppend, chatCatalogPromptMaxLen) {
			return
		}
		params.PromptAppend = *req.PromptAppend
	}
	if req.ModelConfigID != nil {
		if *req.ModelConfigID == uuid.Nil {
			params.ModelConfigID = uuid.NullUUID{}
		} else {
			if !api.validateChatCatalogModelConfigID(rw, r, *req.ModelConfigID) {
				return
			}
			params.ModelConfigID = uuid.NullUUID{UUID: *req.ModelConfigID, Valid: true}
		}
	}
	if req.Enabled != nil {
		params.Enabled = *req.Enabled
	}

	updated, err := api.Database.UpdateChatAgent(ctx, params)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to update chat agent.",
			Detail:  err.Error(),
		})
		return
	}
	aReq.New = updated

	httpapi.Write(ctx, rw, http.StatusOK, convertChatAgent(updated))
}

// DeleteChatAgent is registered by enterprise coderd behind a premium
// feature gate.
//
// @Summary Delete chat agent
// @ID delete-chat-agent
// @Security CoderSessionToken
// @Tags Chats
// @Param agent path string true "Chat agent ID" format(uuid)
// @Success 204
// @Router /api/experimental/chats/agents/{agent} [delete]
// @Description Experimental: this endpoint is subject to change.
func (api *API) DeleteChatAgent(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	agentID, ok := parseChatAgentID(rw, r)
	if !ok {
		return
	}

	existing, err := api.Database.GetChatAgentByID(ctx, agentID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to get chat agent.",
			Detail:  err.Error(),
		})
		return
	}
	if existing.Builtin {
		httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
			Message: "Builtin agents cannot be deleted.",
		})
		return
	}

	aReq, commitAudit := audit.InitRequest[database.ChatAgent](rw, &audit.RequestParams{
		Audit:          *api.Auditor.Load(),
		Log:            api.Logger,
		Request:        r,
		Action:         database.AuditActionDelete,
		OrganizationID: existing.OrganizationID.UUID,
	})
	defer commitAudit()
	aReq.Old = existing

	err = api.Database.UpdateChatAgentDeletedByID(ctx, agentID)
	if err != nil {
		if httpapi.Is404Error(err) {
			httpapi.ResourceNotFound(rw)
			return
		}
		httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Failed to delete chat agent.",
			Detail:  err.Error(),
		})
		return
	}

	rw.WriteHeader(http.StatusNoContent)
}

func parseChatPersonaID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	personaID, err := uuid.Parse(chi.URLParam(r, "persona"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat persona ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return personaID, true
}

func parseChatAgentID(rw http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agent"))
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusBadRequest, codersdk.Response{
			Message: "Invalid chat agent ID.",
			Detail:  err.Error(),
		})
		return uuid.Nil, false
	}
	return agentID, true
}

// resolveCreateChatAgent resolves the agent selected for chat creation
// along with its persona. Lookups go through the caller's dbauthz
// context so read access is enforced. The
// agent must be enabled and be builtin, deployment-scoped, or belong
// to the chat's organization; its persona must be enabled and follow
// the same scope rules.
func (api *API) resolveCreateChatAgent(
	ctx context.Context,
	organizationID uuid.UUID,
	agentID uuid.UUID,
) (database.ChatAgent, database.ChatPersona, int, *codersdk.Response) {
	agent, err := api.Database.GetChatAgentByID(ctx, agentID)
	if err != nil {
		if httpapi.Is404Error(err) {
			return database.ChatAgent{}, database.ChatPersona{}, http.StatusBadRequest, &codersdk.Response{
				Message: "Chat agent does not exist.",
			}
		}
		return database.ChatAgent{}, database.ChatPersona{}, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to resolve chat agent.",
			Detail:  err.Error(),
		}
	}
	if !agent.Enabled {
		return database.ChatAgent{}, database.ChatPersona{}, http.StatusBadRequest, &codersdk.Response{
			Message: "Chat agent is disabled.",
		}
	}
	if agent.OrganizationID.Valid && agent.OrganizationID.UUID != organizationID {
		return database.ChatAgent{}, database.ChatPersona{}, http.StatusBadRequest, &codersdk.Response{
			Message: "Chat agent belongs to a different organization.",
		}
	}

	persona, err := api.Database.GetChatPersonaByID(ctx, agent.PersonaID)
	if err != nil {
		if httpapi.Is404Error(err) {
			return database.ChatAgent{}, database.ChatPersona{}, http.StatusBadRequest, &codersdk.Response{
				Message: "The chat agent's persona does not exist.",
			}
		}
		return database.ChatAgent{}, database.ChatPersona{}, http.StatusInternalServerError, &codersdk.Response{
			Message: "Failed to resolve chat persona.",
			Detail:  err.Error(),
		}
	}
	if !persona.Enabled {
		return database.ChatAgent{}, database.ChatPersona{}, http.StatusBadRequest, &codersdk.Response{
			Message: "The chat agent's persona is disabled.",
		}
	}
	if persona.OrganizationID.Valid && persona.OrganizationID.UUID != organizationID {
		return database.ChatAgent{}, database.ChatPersona{}, http.StatusBadRequest, &codersdk.Response{
			Message: "The chat agent's persona belongs to a different organization.",
		}
	}
	return agent, persona, 0, nil
}

// populateChatAgentSummaries fills slug, name, icon, and the builtin
// flag on the ID-only agent summaries that db2sdk.Chat attaches to
// chats created as an agent, including their children. Agent rows are
// collected and fetched in a single batch. Soft-deleted agents still
// resolve so attribution survives deletion. Failures are non-fatal:
// the summary keeps its ID.
func (api *API) populateChatAgentSummaries(ctx context.Context, chats ...*codersdk.Chat) {
	var pending []*codersdk.ChatAgentSummary
	var collect func(chat *codersdk.Chat)
	collect = func(chat *codersdk.Chat) {
		if chat.Agent != nil {
			pending = append(pending, chat.Agent)
		}
		for i := range chat.Children {
			collect(&chat.Children[i])
		}
	}
	for _, chat := range chats {
		if chat != nil {
			collect(chat)
		}
	}
	if len(pending) == 0 {
		return
	}

	ids := make([]uuid.UUID, 0, len(pending))
	seen := make(map[uuid.UUID]struct{}, len(pending))
	for _, summary := range pending {
		if _, ok := seen[summary.ID]; ok {
			continue
		}
		seen[summary.ID] = struct{}{}
		ids = append(ids, summary.ID)
	}
	// The chat rows themselves were already authorized; the agent
	// summary is display metadata every member may read anyway.
	//nolint:gocritic // See above.
	agents, err := api.Database.GetChatAgentsByIDs(dbauthz.AsChatd(ctx), ids)
	if err != nil {
		api.Logger.Warn(ctx, "failed to resolve chat agent summaries", slog.Error(err))
		return
	}
	byID := make(map[uuid.UUID]database.ChatAgent, len(agents))
	for _, agent := range agents {
		byID[agent.ID] = agent
	}
	for _, summary := range pending {
		agent, ok := byID[summary.ID]
		if !ok {
			continue
		}
		summary.Slug = agent.Slug
		summary.Name = agent.Name
		summary.Icon = agent.Icon
		summary.Builtin = agent.Builtin
	}
}
