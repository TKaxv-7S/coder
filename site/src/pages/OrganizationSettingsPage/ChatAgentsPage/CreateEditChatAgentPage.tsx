import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import {
	chatAgents,
	chatPersonas,
	createChatAgent,
	NIL_UUID,
	updateChatAgent,
} from "#/api/queries/chatAgents";
import { chatModelConfigs } from "#/api/queries/chats";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import {
	SettingsHeader,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	ChatAgentForm,
	type ChatAgentFormValues,
} from "#/modules/chatAgents/ChatAgentForm";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";

const CreateEditChatAgentPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { organization: organizationName, agentId } = useParams<{
		organization: string;
		agentId: string;
	}>();

	const agentsQuery = useQuery(chatAgents(organization?.id));
	const personasQuery = useQuery(chatPersonas(organization?.id));
	const modelConfigsQuery = useQuery(chatModelConfigs());

	const createMutation = useMutation(createChatAgent(queryClient));
	const updateMutation = useMutation(updateChatAgent(queryClient));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const editingAgent = agentId
		? agentsQuery.data?.find((agent) => agent.id === agentId)
		: undefined;
	const isEditing = Boolean(agentId);
	// Builtin and deployment agents are shown read-only for reference.
	const isReadOnly = Boolean(
		editingAgent &&
			(editingAgent.builtin ||
				editingAgent.organization_id !== organization.id),
	);
	const listPath = `/organizations/${organizationName}/chat-agents`;

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
					organization_id: organization.id,
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
		<RequirePermission
			isFeatureVisible={organizationPermissions?.createChatAgents ?? false}
		>
			<title>
				{pageTitle(
					isEditing ? "Edit agent" : "Create agent",
					organization.display_name || organization.name,
				)}
			</title>

			<SettingsHeader>
				<SettingsHeaderTitle>
					{isReadOnly
						? "View agent"
						: isEditing
							? "Edit agent"
							: "Create agent"}
				</SettingsHeaderTitle>
			</SettingsHeader>
			{isEditing && agentsQuery.isLoading ? (
				<Loader />
			) : isEditing && agentsQuery.isError ? (
				<ErrorAlert error={agentsQuery.error} />
			) : isEditing && !editingAgent ? (
				<EmptyState message="Agent not found" />
			) : (
				<ChatAgentForm
					editingAgent={editingAgent}
					personas={personasQuery.data ?? []}
					modelConfigs={modelConfigsQuery.data ?? []}
					isSaving={createMutation.isPending || updateMutation.isPending}
					readOnly={isReadOnly}
					error={
						personasQuery.error ??
						modelConfigsQuery.error ??
						createMutation.error ??
						updateMutation.error
					}
					onSubmit={handleSubmit}
					onCancel={() => void navigate(listPath)}
				/>
			)}
		</RequirePermission>
	);
};

export default CreateEditChatAgentPage;
