# Personas and agents

Administrators can define named personas and agents that control how Coder
Agents chats behave:

- A **persona** bundles a system prompt with an optional preferred model.
- An **agent** points at a persona and optionally appends extra prompt text
  or overrides the model.

Users pick an agent when starting a chat (or through the API), and the chat
runs with that agent's persona prompt and model instead of the defaults.

> [!NOTE]
> Creating, updating, and deleting personas and agents requires a
> [Premium license](https://coder.com/pricing#compare-plans). Listing them,
> the builtin entries, and starting chats as an agent work on every
> deployment.

## Scopes

Personas and agents exist at three scopes. Slugs are unique within each
scope, and the management UI and API address entries by ID, so entries
from all scopes appear side by side. When scopes reuse a slug, the
`spawn_agent` delegation described below resolves the slug with a fixed
precedence: builtin, then organization, then deployment.

| Scope        | Managed by  | Visible to           | Notes                                      |
|--------------|-------------|----------------------|--------------------------------------------|
| Builtin      | Nobody      | Everyone             | Shipped with Coder. Cannot be edited.      |
| Deployment   | Site owners | Everyone             | Available in every organization.           |
| Organization | Org admins  | Organization members | Usable only by chats in that organization. |

The builtin catalog ships three personas (`swe`, `general-assistant`, and
`code-reviewer`) and three agents (`Coder`, `Assistant`, and `Reviewer`).
The `Coder` agent runs the `swe` persona and is the implicit default when a
chat is created without an agent, which preserves the standard Coder Agents
behavior.

## How personas affect chats

- The persona's system prompt replaces the built-in default prompt as the
  base of the chat's system prompt. The deployment-level
  [custom system prompt](../getting-started.md) setting still appends its
  custom text, but its "include default prompt" toggle governs only the
  built-in default: an explicitly selected persona prompt is always
  included.
- The agent's prompt append, when set, is added as an additional system
  message directly after the persona prompt.
- The model is chosen in this order: a model explicitly selected for the
  chat, then the agent's model override, then the persona's preferred
  model, then the deployment's normal model resolution. An unavailable
  preference falls through to the next tier instead of failing the chat.
- Prompts are captured in the chat's history at creation, so existing chats
  are not affected by later persona or agent edits. Deletes are soft:
  existing chats keep their agent attribution.

## Manage personas and agents

Deployment-scoped entries live at **AI Settings** > **Coder Agents** >
**Personas** (`/ai/settings/personas`) and **Agents**
(`/ai/settings/agents`). Organization-scoped entries live in
**Organization settings** > **Chat Personas** and **Chat Agents**, visible
to organization admins.

Each persona has a slug, name, description, icon, system prompt, and an
optional preferred model. Each agent has the same identity fields plus the
persona it runs, an optional prompt append, and an optional model override.
An organization-scoped agent may reference a builtin or deployment-scoped
persona, but not a persona from another organization. A persona cannot be
deleted while agents still reference it. Disabled entries
stay listed for management but cannot be used in new chats.

> [!NOTE]
> Persona system prompts and agent prompt appends are returned verbatim
> to every user who can list them. Do not embed secrets, credentials, or
> confidential details in prompts.

## Use an agent in a chat

In the UI, pick the agent from the agent selector in the chat input when
starting a chat. Through the API, pass the agent's ID when creating a chat.

> [!NOTE]
> These endpoints are experimental. They live under `/api/experimental` and
> may change without notice.

List the agents available to you, optionally including an organization's
agents:

```text
GET /api/experimental/chats/agents?organization={organization_id}
```

The response includes builtin, deployment, and (when requested)
organization agents, each with its `id`, `slug`, `persona_id`, and
`builtin` flag. Personas are listed the same way at
`GET /api/experimental/chats/personas`.

Create a chat as an agent by setting `chat_agent_id`:

```sh
curl -X POST https://coder.example.com/api/experimental/chats \
  -H "Coder-Session-Token: $CODER_SESSION_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "organization_id": "<organization-uuid>",
    "chat_agent_id": "<agent-uuid>",
    "content": [
      {"type": "text", "text": "Review the open pull request"}
    ]
  }'
```

The response and later reads include an `agent` summary (ID, slug, name,
icon, and builtin flag) for attribution. Omitting `chat_agent_id` keeps
today's default behavior.

Inside a running chat, the model can delegate work to these agents: the
`spawn_agent` tool accepts `agent:<slug>` type values (for example,
`agent:reviewer`) in addition to its builtin subagent types, and the child
chat runs with that agent's persona and instructions.
