import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type { CreateExternalAuthProviderRequest } from "#/api/typesGenerated";

const providerConfigsKey = ["externalAuthProviderConfigs"];

export const externalAuthProviderConfigs = () => ({
	queryKey: providerConfigsKey,
	queryFn: () => API.getExternalAuthProviderConfigs(),
});

export const createExternalAuthProviderConfig = (queryClient: QueryClient) => ({
	mutationFn: (req: CreateExternalAuthProviderRequest) =>
		API.createExternalAuthProviderConfig(req),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: providerConfigsKey });
	},
});

export const deleteExternalAuthProviderConfig = (queryClient: QueryClient) => ({
	mutationFn: (id: string) => API.deleteExternalAuthProviderConfig(id),
	onSuccess: async () => {
		await queryClient.invalidateQueries({ queryKey: providerConfigsKey });
	},
});
