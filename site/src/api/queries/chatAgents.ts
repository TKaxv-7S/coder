import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

// NIL_UUID clears a persona or agent model preference on update; the
// API treats the zero UUID as "remove the preference".
export const NIL_UUID = "00000000-0000-0000-0000-000000000000";

// BUILTIN_CODER_AGENT_SLUG is the slug of the builtin default Coder
// agent (defined in coderd/x/chatd/builtin_agents.go). Chats created
// as this agent match the no-agent default behavior, so the UI treats
// it as the implicit default and hides attribution for it.
const BUILTIN_CODER_AGENT_SLUG = "coder";

// isDefaultCoderAgent reports whether an agent summary refers to the
// builtin default Coder agent.
export const isDefaultCoderAgent = (agent: {
	slug?: string;
	builtin?: boolean;
}): boolean =>
	Boolean(agent.builtin) && agent.slug === BUILTIN_CODER_AGENT_SLUG;

const chatPersonasKey = (organizationId?: string) =>
	["chat-personas", organizationId ?? "deployment"] as const;

export const chatPersonas = (organizationId?: string) => ({
	queryKey: chatPersonasKey(organizationId),
	queryFn: (): Promise<TypesGen.ChatPersona[]> =>
		API.experimental.getChatPersonas(organizationId),
});

const chatAgentsKey = (organizationId?: string) =>
	["chat-agents", organizationId ?? "deployment"] as const;

export const chatAgents = (organizationId?: string) => ({
	queryKey: chatAgentsKey(organizationId),
	queryFn: (): Promise<TypesGen.ChatAgent[]> =>
		API.experimental.getChatAgents(organizationId),
});

// Agent rows join persona names in the UI and persona deletion is
// blocked by referencing agents, so changes to either list can affect
// how the other renders. Invalidate both key families.
const invalidateChatAgentQueries = async (queryClient: QueryClient) => {
	await Promise.all([
		queryClient.invalidateQueries({ queryKey: ["chat-personas"] }),
		queryClient.invalidateQueries({ queryKey: ["chat-agents"] }),
	]);
};

export const createChatPersona = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatPersonaRequest) =>
		API.experimental.createChatPersona(req),
	onSuccess: async () => {
		await invalidateChatAgentQueries(queryClient);
	},
});

type UpdateChatPersonaArgs = {
	personaId: string;
	req: TypesGen.UpdateChatPersonaRequest;
};

export const updateChatPersona = (queryClient: QueryClient) => ({
	mutationFn: ({ personaId, req }: UpdateChatPersonaArgs) =>
		API.experimental.updateChatPersona(personaId, req),
	onSuccess: async () => {
		await invalidateChatAgentQueries(queryClient);
	},
});

export const deleteChatPersona = (queryClient: QueryClient) => ({
	mutationFn: (personaId: string) =>
		API.experimental.deleteChatPersona(personaId),
	onSuccess: async () => {
		await invalidateChatAgentQueries(queryClient);
	},
});

export const createChatAgent = (queryClient: QueryClient) => ({
	mutationFn: (req: TypesGen.CreateChatAgentRequest) =>
		API.experimental.createChatAgent(req),
	onSuccess: async () => {
		await invalidateChatAgentQueries(queryClient);
	},
});

type UpdateChatAgentArgs = {
	agentId: string;
	req: TypesGen.UpdateChatAgentRequest;
};

export const updateChatAgent = (queryClient: QueryClient) => ({
	mutationFn: ({ agentId, req }: UpdateChatAgentArgs) =>
		API.experimental.updateChatAgent(agentId, req),
	onSuccess: async () => {
		await invalidateChatAgentQueries(queryClient);
	},
});

export const deleteChatAgent = (queryClient: QueryClient) => ({
	mutationFn: (agentId: string) => API.experimental.deleteChatAgent(agentId),
	onSuccess: async () => {
		await invalidateChatAgentQueries(queryClient);
	},
});
