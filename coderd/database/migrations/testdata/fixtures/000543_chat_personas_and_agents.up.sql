INSERT INTO chat_personas (
    id,
    organization_id,
    slug,
    name,
    description,
    icon,
    system_prompt,
    model_config_id,
    enabled,
    deleted,
    created_by,
    created_at,
    updated_at
) VALUES (
    '7d5f6b6c-95a1-4e3a-b1f0-1c2d3e4f5a6b',
    NULL, -- deployment scope
    'fixture-persona',
    'Fixture Persona',
    'A fixture persona for migration tests.',
    '/emojis/1f916.png',
    'You are a fixture persona.',
    NULL,
    TRUE,
    FALSE,
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);

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
    created_by,
    created_at,
    updated_at
) VALUES (
    '8e6a7c7d-a6b2-4f4b-c2a1-2d3e4f5a6b7c',
    NULL, -- deployment scope
    'fixture-agent',
    'Fixture Agent',
    'A fixture agent for migration tests.',
    '/emojis/1f9be.png',
    '7d5f6b6c-95a1-4e3a-b1f0-1c2d3e4f5a6b',
    'Additional fixture instructions.',
    NULL,
    TRUE,
    FALSE,
    '30095c71-380b-457a-8995-97b8ee6e5307', -- admin@coder.com
    '2024-01-01 00:00:00+00',
    '2024-01-01 00:00:00+00'
);
