-- name: GetChatAgentByID :one
SELECT
    *
FROM
    chat_agents
WHERE
    id = @id::uuid
    AND deleted = FALSE;

-- name: GetChatAgentsByIDs :many
-- Includes soft-deleted rows so chats keep their agent attribution
-- after the agent is deleted.
SELECT
    *
FROM
    chat_agents
WHERE
    id = ANY(@ids::uuid[]);

-- name: GetChatAgents :many
-- Returns deployment-scoped agents and, when an organization ID is
-- given, that organization's agents as well.
SELECT
    *
FROM
    chat_agents
WHERE
    deleted = FALSE
    AND (
        organization_id IS NULL
        OR (
            @organization_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid
            AND organization_id = @organization_id::uuid
        )
    )
ORDER BY
    (organization_id IS NULL) DESC,
    slug ASC;

-- name: InsertChatAgent :one
INSERT INTO chat_agents (
    organization_id,
    slug,
    name,
    description,
    icon,
    persona_id,
    prompt_append,
    model_config_id,
    enabled,
    created_by
)
VALUES (
    @organization_id,
    @slug,
    @name,
    @description,
    @icon,
    @persona_id,
    @prompt_append,
    @model_config_id,
    @enabled,
    @created_by
)
RETURNING *;

-- name: UpdateChatAgent :one
UPDATE
    chat_agents
SET
    name = @name,
    description = @description,
    icon = @icon,
    persona_id = @persona_id,
    prompt_append = @prompt_append,
    model_config_id = @model_config_id,
    enabled = @enabled,
    updated_at = now()
WHERE
    id = @id::uuid
    AND deleted = FALSE
RETURNING *;

-- name: UpdateChatAgentDeletedByID :exec
-- Soft delete keeps the row so attribution lookups on existing chats
-- (GetChatAgentsByIDs) still resolve the agent's identity and the
-- foreign key from chats.chat_agent_id remains satisfied.
UPDATE
    chat_agents
SET
    deleted = TRUE,
    updated_at = now()
WHERE
    id = @id::uuid;

-- name: CountChatAgentsByPersonaID :one
-- Counts non-deleted agents referencing a persona. Used to block
-- persona deletion while agents still depend on it; the foreign key
-- alone cannot enforce this because deletion is a soft delete.
SELECT
    COUNT(*)
FROM
    chat_agents
WHERE
    persona_id = @persona_id::uuid
    AND deleted = FALSE;

-- name: UpsertBuiltinChatAgent :exec
-- Seeds or refreshes a builtin agent row at coderd startup. Builtin
-- rows carry the canonical in-repo values, are always enabled and
-- undeleted, and have no creator.
INSERT INTO chat_agents (
    id,
    organization_id,
    slug,
    name,
    description,
    icon,
    persona_id,
    prompt_append,
    model_config_id,
    enabled,
    deleted,
    builtin,
    created_by
)
VALUES (
    @id,
    NULL,
    @slug,
    @name,
    @description,
    @icon,
    @persona_id,
    @prompt_append,
    NULL,
    TRUE,
    FALSE,
    TRUE,
    NULL
)
ON CONFLICT (id) DO UPDATE SET
    slug = EXCLUDED.slug,
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    icon = EXCLUDED.icon,
    persona_id = EXCLUDED.persona_id,
    prompt_append = EXCLUDED.prompt_append,
    enabled = TRUE,
    deleted = FALSE,
    builtin = TRUE,
    updated_at = now();
