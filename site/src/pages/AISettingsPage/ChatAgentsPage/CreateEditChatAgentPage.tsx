import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import {
	chatAgents,
	chatPersonas,
	createChatAgent,
	updateChatAgent,
} from "#/api/queries/chatAgents";
import { chatModelConfigs } from "#/api/queries/chats";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	ChatAgentForm,
	type ChatAgentFormValues,
} from "#/modules/chatAgents/ChatAgentForm";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { NIL_UUID } from "../ChatPersonasPage/CreateEditChatPersonaPage";

const CreateEditChatAgentPage: FC = () => {
	const { permissions } = useAuthenticated();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { agentId } = useParams() as { agentId?: string };

	const agentsQuery = useQuery(chatAgents());
	const personasQuery = useQuery(chatPersonas());
	const modelConfigsQuery = useQuery(chatModelConfigs());

	const createMutation = useMutation(createChatAgent(queryClient));
	const updateMutation = useMutation(updateChatAgent(queryClient));

	const editingAgent = agentId
		? agentsQuery.data?.find((agent) => agent.id === agentId)
		: undefined;
	const isEditing = Boolean(agentId);
	const isBuiltin = Boolean(editingAgent?.builtin);
	const listPath = "/ai/settings/agents";

	const handleSubmit = async (values: ChatAgentFormValues) => {
		try {
			if (isEditing && editingAgent) {
				await updateMutation.mutateAsync({
					agentId: editingAgent.id,
					req: {
						name: values.name,
						description: values.description,
						icon: values.icon,
						persona_id: values.persona_id,
						prompt_append: values.prompt_append,
						model_config_id: values.model_config_id || NIL_UUID,
						enabled: values.enabled,
					},
				});
				toast.success(`Agent "${values.name}" updated.`);
			} else {
				await createMutation.mutateAsync({
					slug: values.slug,
					name: values.name,
					description: values.description,
					icon: values.icon,
					persona_id: values.persona_id,
					prompt_append: values.prompt_append,
					model_config_id: values.model_config_id || undefined,
					enabled: values.enabled,
				});
				toast.success(`Agent "${values.name}" created.`);
			}
			void navigate(listPath);
		} catch {
			// The mutation error renders inline in the form.
		}
	};

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>
				{pageTitle(isEditing ? "Edit agent" : "Create agent", "AI Settings")}
			</title>

			<SettingsHeader>
				<SettingsHeaderTitle>
					{isBuiltin ? "View agent" : isEditing ? "Edit agent" : "Create agent"}
				</SettingsHeaderTitle>
			</SettingsHeader>
			{isEditing && agentsQuery.isLoading ? (
				<Loader />
			) : isEditing && !editingAgent ? (
				<EmptyState message="Agent not found" />
			) : (
				<ChatAgentForm
					editingAgent={editingAgent}
					personas={personasQuery.data ?? []}
					modelConfigs={modelConfigsQuery.data ?? []}
					isSaving={createMutation.isPending || updateMutation.isPending}
					readOnly={isBuiltin}
					error={createMutation.error ?? updateMutation.error}
					onSubmit={handleSubmit}
					onCancel={() => void navigate(listPath)}
				/>
			)}
		</RequirePermission>
	);
};

export default CreateEditChatAgentPage;
