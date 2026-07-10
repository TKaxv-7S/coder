import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import {
	chatPersonas,
	createChatPersona,
	updateChatPersona,
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
	ChatPersonaForm,
	type ChatPersonaFormValues,
} from "#/modules/chatAgents/ChatPersonaForm";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";

// UpdateChatPersonaRequest treats the zero UUID as "clear the model
// preference"; omitting the field leaves it unchanged.
export const NIL_UUID = "00000000-0000-0000-0000-000000000000";

const CreateEditChatPersonaPage: FC = () => {
	const { permissions } = useAuthenticated();
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { personaId } = useParams() as { personaId?: string };

	const personasQuery = useQuery(chatPersonas());
	const modelConfigsQuery = useQuery(chatModelConfigs());

	const createMutation = useMutation(createChatPersona(queryClient));
	const updateMutation = useMutation(updateChatPersona(queryClient));

	const editingPersona = personaId
		? personasQuery.data?.find((persona) => persona.id === personaId)
		: undefined;
	const isEditing = Boolean(personaId);
	const isBuiltin = Boolean(editingPersona?.builtin);
	const listPath = "/ai/settings/personas";

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
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>
				{pageTitle(
					isEditing ? "Edit persona" : "Create persona",
					"AI Settings",
				)}
			</title>

			<SettingsHeader>
				<SettingsHeaderTitle>
					{isBuiltin
						? "View persona"
						: isEditing
							? "Edit persona"
							: "Create persona"}
				</SettingsHeaderTitle>
			</SettingsHeader>
			{isEditing && personasQuery.isLoading ? (
				<Loader />
			) : isEditing && !editingPersona ? (
				<EmptyState message="Persona not found" />
			) : (
				<ChatPersonaForm
					editingPersona={editingPersona}
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

export default CreateEditChatPersonaPage;
