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
-- Soft delete keeps the row so chats referencing the agent retain FK
-- integrity.
UPDATE
    chat_agents
SET
    deleted = TRUE,
    updated_at = now()
WHERE
    id = @id::uuid;
