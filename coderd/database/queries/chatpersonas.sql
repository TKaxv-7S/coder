-- name: GetChatPersonaByID :one
SELECT
    *
FROM
    chat_personas
WHERE
    id = @id::uuid
    AND deleted = FALSE;

-- name: GetChatPersonas :many
-- Returns deployment-scoped personas and, when an organization ID is
-- given, that organization's personas as well.
SELECT
    *
FROM
    chat_personas
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

-- name: InsertChatPersona :one
INSERT INTO chat_personas (
    organization_id,
    slug,
    name,
    description,
    icon,
    system_prompt,
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
    @system_prompt,
    @model_config_id,
    @enabled,
    @created_by
)
RETURNING *;

-- name: UpdateChatPersona :one
UPDATE
    chat_personas
SET
    name = @name,
    description = @description,
    icon = @icon,
    system_prompt = @system_prompt,
    model_config_id = @model_config_id,
    enabled = @enabled,
    updated_at = now()
WHERE
    id = @id::uuid
    AND deleted = FALSE
RETURNING *;

-- name: UpdateChatPersonaDeletedByID :exec
-- Soft delete keeps the row so agents and chats referencing the
-- persona retain FK integrity.
UPDATE
    chat_personas
SET
    deleted = TRUE,
    updated_at = now()
WHERE
    id = @id::uuid;
