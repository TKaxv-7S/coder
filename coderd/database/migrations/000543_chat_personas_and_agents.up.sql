-- Chat personas bundle a system prompt with a preferred model. A NULL
-- organization_id means the persona is deployment-scoped. Builtin
-- personas are deployment-scoped rows seeded (and refreshed) at coderd
-- startup from the in-repo catalog; they have no creator and cannot be
-- modified or deleted through the API.
CREATE TABLE chat_personas (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID        REFERENCES organizations(id) ON DELETE CASCADE,
    slug            TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    icon            TEXT        NOT NULL DEFAULT '',
    system_prompt   TEXT        NOT NULL,
    model_config_id UUID        REFERENCES chat_model_configs(id),
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    deleted         BOOLEAN     NOT NULL DEFAULT FALSE,
    builtin         BOOLEAN     NOT NULL DEFAULT FALSE,
    created_by      UUID        REFERENCES users(id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN chat_personas.organization_id IS 'NULL means the persona is deployment-scoped; otherwise it belongs to the organization.';
COMMENT ON COLUMN chat_personas.model_config_id IS 'Preferred model for chats created with this persona. NULL falls back to the default model resolution.';
COMMENT ON COLUMN chat_personas.builtin IS 'Builtin rows are seeded at startup from the in-repo catalog and are immutable through the API.';
COMMENT ON COLUMN chat_personas.created_by IS 'NULL for builtin rows seeded at startup.';

-- Slugs are unique per scope among non-deleted rows. Deployment scope
-- (NULL organization_id) needs its own index because NULLs are
-- distinct in composite unique indexes.
CREATE UNIQUE INDEX idx_chat_personas_org_slug
    ON chat_personas (organization_id, slug)
    WHERE NOT deleted AND organization_id IS NOT NULL;
CREATE UNIQUE INDEX idx_chat_personas_deployment_slug
    ON chat_personas (slug)
    WHERE NOT deleted AND organization_id IS NULL;

-- Chat agents are named invocable entries that point at a persona and
-- optionally append to its prompt or override its model. Builtin
-- agents are deployment-scoped rows seeded at coderd startup, like
-- builtin personas.
CREATE TABLE chat_agents (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID        REFERENCES organizations(id) ON DELETE CASCADE,
    slug            TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    icon            TEXT        NOT NULL DEFAULT '',
    persona_id      UUID        NOT NULL REFERENCES chat_personas(id),
    prompt_append   TEXT        NOT NULL DEFAULT '',
    model_config_id UUID        REFERENCES chat_model_configs(id),
    enabled         BOOLEAN     NOT NULL DEFAULT TRUE,
    deleted         BOOLEAN     NOT NULL DEFAULT FALSE,
    builtin         BOOLEAN     NOT NULL DEFAULT FALSE,
    created_by      UUID        REFERENCES users(id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON COLUMN chat_agents.organization_id IS 'NULL means the agent is deployment-scoped; otherwise it belongs to the organization.';
COMMENT ON COLUMN chat_agents.persona_id IS 'The persona supplying the base system prompt for chats created with this agent.';
COMMENT ON COLUMN chat_agents.prompt_append IS 'Additional system prompt text appended after the persona prompt.';
COMMENT ON COLUMN chat_agents.model_config_id IS 'Overrides the persona model preference when set.';
COMMENT ON COLUMN chat_agents.builtin IS 'Builtin rows are seeded at startup from the in-repo catalog and are immutable through the API.';
COMMENT ON COLUMN chat_agents.created_by IS 'NULL for builtin rows seeded at startup.';

CREATE UNIQUE INDEX idx_chat_agents_org_slug
    ON chat_agents (organization_id, slug)
    WHERE NOT deleted AND organization_id IS NOT NULL;
CREATE UNIQUE INDEX idx_chat_agents_deployment_slug
    ON chat_agents (slug)
    WHERE NOT deleted AND organization_id IS NULL;

-- The chats_expanded view must be dropped and recreated so the new
-- chats column can appear in its column list.
DROP VIEW IF EXISTS chats_expanded;

-- The existing chats.agent_id column is the workspace agent, so the
-- chat agent reference uses a distinct name.
ALTER TABLE chats ADD COLUMN chat_agent_id UUID REFERENCES chat_agents(id);

COMMENT ON COLUMN chats.chat_agent_id IS 'The chat agent the chat was created as, if any. Distinct from agent_id, which is the workspace agent.';

CREATE VIEW chats_expanded AS
 SELECT c.id,
    c.owner_id,
    c.workspace_id,
    c.title,
    c.status,
    c.worker_id,
    c.started_at,
    c.heartbeat_at,
    c.created_at,
    c.updated_at,
    c.parent_chat_id,
    c.root_chat_id,
    c.last_model_config_id,
    c.last_reasoning_effort,
    c.archived,
    c.last_error,
    c.mode,
    c.mcp_server_ids,
    c.labels,
    c.build_id,
    c.agent_id,
    c.chat_agent_id,
    c.pin_order,
    c.last_read_message_id,
    c.dynamic_tools,
    c.organization_id,
    c.plan_mode,
    c.client_type,
    c.last_turn_summary,
    c.snapshot_version,
    c.history_version,
    c.queue_version,
    c.generation_attempt,
    c.retry_state,
    c.retry_state_version,
    c.runner_id,
    c.requires_action_deadline_at,
    COALESCE(root.user_acl, c.user_acl) AS user_acl,
    COALESCE(root.group_acl, c.group_acl) AS group_acl,
    owner.username AS owner_username,
    owner.name AS owner_name,
    c.context_aggregate_hash,
    c.context_dirty_since,
    c.context_dirty_resources,
    c.context_error
   FROM ((chats c
     LEFT JOIN chats root ON ((root.id = COALESCE(c.root_chat_id, c.parent_chat_id))))
     JOIN visible_users owner ON ((owner.id = c.owner_id)));

-- Audit log resource types for the new tables.
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_persona';
ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'chat_agent';

-- API key scopes for the new RBAC resources. These are internal-only
-- (not listed in the external scope catalog), matching chat scopes.
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_persona:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_persona:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_persona:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_persona:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_persona:update';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_agent:*';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_agent:create';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_agent:delete';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_agent:read';
ALTER TYPE api_key_scope ADD VALUE IF NOT EXISTS 'chat_agent:update';
