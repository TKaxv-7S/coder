import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";

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

// Agent list responses embed the effective persona/model, so persona
// changes can affect agent rows. Invalidate both key families.
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
