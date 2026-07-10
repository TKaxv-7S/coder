import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import {
	chatPersonas,
	createChatPersona,
	NIL_UUID,
	updateChatPersona,
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
	ChatPersonaForm,
	type ChatPersonaFormValues,
} from "#/modules/chatAgents/ChatPersonaForm";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";

const CreateEditChatPersonaPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { organization: organizationName, personaId } = useParams<{
		organization: string;
		personaId: string;
	}>();

	const personasQuery = useQuery(chatPersonas(organization?.id));
	const modelConfigsQuery = useQuery(chatModelConfigs());

	const createMutation = useMutation(createChatPersona(queryClient));
	const updateMutation = useMutation(updateChatPersona(queryClient));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const editingPersona = personaId
		? personasQuery.data?.find((persona) => persona.id === personaId)
		: undefined;
	const isEditing = Boolean(personaId);
	// Builtin and deployment personas are shown read-only for reference.
	const isReadOnly = Boolean(
		editingPersona &&
			(editingPersona.builtin ||
				editingPersona.organization_id !== organization.id),
	);
	const listPath = `/organizations/${organizationName}/chat-personas`;

	const handleSubmit = async (values: ChatPersonaFormValues) => {
		try {
			if (isEditing && editingPersona) {
				await updateMutation.mutateAsync({
					personaId: editingPersona.id,
					req: {
						name: values.name,
						description: values.description,
						icon: values.icon,
						system_prompt: values.system_prompt,
						model_config_id: values.model_config_id || NIL_UUID,
						enabled: values.enabled,
					},
				});
				toast.success(`Persona "${values.name}" updated.`);
			} else {
				await createMutation.mutateAsync({
					organization_id: organization.id,
					slug: values.slug,
					name: values.name,
					description: values.description,
					icon: values.icon,
					system_prompt: values.system_prompt,
					model_config_id: values.model_config_id || undefined,
					enabled: values.enabled,
				});
				toast.success(`Persona "${values.name}" created.`);
			}
			void navigate(listPath);
		} catch {
			// The mutation error renders inline in the form.
		}
	};

	return (
		<RequirePermission
			isFeatureVisible={organizationPermissions?.createChatPersonas ?? false}
		>
			<title>
				{pageTitle(
					isEditing ? "Edit persona" : "Create persona",
					organization.display_name || organization.name,
				)}
			</title>

			<SettingsHeader>
				<SettingsHeaderTitle>
					{isReadOnly
						? "View persona"
						: isEditing
							? "Edit persona"
							: "Create persona"}
				</SettingsHeaderTitle>
			</SettingsHeader>
			{isEditing && personasQuery.isLoading ? (
				<Loader />
			) : isEditing && personasQuery.isError ? (
				<ErrorAlert error={personasQuery.error} />
			) : isEditing && !editingPersona ? (
				<EmptyState message="Persona not found" />
			) : (
				<ChatPersonaForm
					editingPersona={editingPersona}
					modelConfigs={modelConfigsQuery.data ?? []}
					isSaving={createMutation.isPending || updateMutation.isPending}
					readOnly={isReadOnly}
					error={
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

export default CreateEditChatPersonaPage;
