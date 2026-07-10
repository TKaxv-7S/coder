import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { chatPersonas, deleteChatPersona } from "#/api/queries/chatAgents";
import type { ChatPersona } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { ChatPersonasPageView } from "#/modules/chatAgents/ChatPersonasPageView";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";

const ChatPersonasPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const { chat_agents: isChatAgentsEnabled } = useFeatureVisibility();
	const [personaToDelete, setPersonaToDelete] = useState<ChatPersona>();

	const personasQuery = useQuery(chatPersonas());
	const deleteMutation = useMutation(deleteChatPersona(queryClient));

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("Personas", "AI Settings")}</title>

			<ChatPersonasPageView
				personas={personasQuery.data}
				error={personasQuery.error}
				canEdit={permissions.editDeploymentConfig}
				isEntitled={isChatAgentsEnabled}
				onDelete={setPersonaToDelete}
			/>

			<DeleteDialog
				key={personaToDelete?.id}
				isOpen={personaToDelete !== undefined}
				confirmLoading={deleteMutation.isPending}
				name={personaToDelete?.name ?? ""}
				entity="persona"
				onCancel={() => setPersonaToDelete(undefined)}
				onConfirm={async () => {
					if (!personaToDelete) {
						return;
					}
					try {
						await deleteMutation.mutateAsync(personaToDelete.id);
						toast.success(`Persona "${personaToDelete.name}" deleted.`);
						setPersonaToDelete(undefined);
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to delete persona."), {
							description: getErrorDetail(error),
						});
					}
				}}
			/>
		</RequirePermission>
	);
};

export default ChatPersonasPage;
