import type { ChatAgent, ChatPersona } from "#/api/typesGenerated";
import { MOCK_TIMESTAMP } from "./chatEntities";
import { MockOrganization } from "./entities";

export const MockBuiltinChatPersona: ChatPersona = {
	id: "c0defade-0000-4000-8000-000000000001",
	slug: "swe",
	name: "Software Engineer",
	description:
		"The default software-engineering persona used by the Coder agent.",
	icon: "",
	system_prompt: "You are a helpful software engineering assistant.",
	builtin: true,
	enabled: true,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockDeploymentChatPersona: ChatPersona = {
	id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
	slug: "support",
	name: "Support Engineer",
	description: "Answers support questions.",
	icon: "/emojis/1f9d1-200d-1f4bb.png",
	system_prompt: "You are a support engineer.",
	model_config_id: "model-1",
	builtin: false,
	enabled: true,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockOrganizationChatPersona: ChatPersona = {
	id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
	organization_id: MockOrganization.id,
	slug: "org-writer",
	name: "Org Writer",
	description: "Writes docs for the org.",
	icon: "",
	system_prompt: "You write documentation.",
	builtin: false,
	enabled: false,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockBuiltinChatAgent: ChatAgent = {
	id: "c0defade-0000-4000-8000-000000000101",
	slug: "coder",
	name: "Coder",
	description: "The default Coder software-engineering agent.",
	icon: "",
	persona_id: MockBuiltinChatPersona.id,
	prompt_append: "",
	builtin: true,
	enabled: true,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockBuiltinAssistantChatAgent: ChatAgent = {
	id: "c0defade-0000-4000-8000-000000000102",
	slug: "assistant",
	name: "Assistant",
	description: "A general-purpose assistant.",
	icon: "",
	persona_id: MockBuiltinChatPersona.id,
	prompt_append: "",
	builtin: true,
	enabled: true,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockDeploymentChatAgent: ChatAgent = {
	id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1",
	slug: "support",
	name: "Support",
	description: "Handles support requests.",
	icon: "/emojis/1f9d1-200d-1f4bb.png",
	persona_id: MockDeploymentChatPersona.id,
	prompt_append: "Always link to the docs.",
	builtin: false,
	enabled: true,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockOrganizationChatAgent: ChatAgent = {
	id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2",
	organization_id: MockOrganization.id,
	slug: "org-writer",
	name: "Org Writer",
	description: "Writes docs for the org.",
	icon: "",
	persona_id: MockOrganizationChatPersona.id,
	prompt_append: "",
	model_config_id: "model-1",
	builtin: false,
	enabled: false,
	created_at: MOCK_TIMESTAMP,
	updated_at: MOCK_TIMESTAMP,
};

export const MockChatPersonas: ChatPersona[] = [
	MockBuiltinChatPersona,
	MockDeploymentChatPersona,
	MockOrganizationChatPersona,
];

export const MockChatAgents: ChatAgent[] = [
	MockBuiltinChatAgent,
	MockBuiltinAssistantChatAgent,
	MockDeploymentChatAgent,
	MockOrganizationChatAgent,
];
