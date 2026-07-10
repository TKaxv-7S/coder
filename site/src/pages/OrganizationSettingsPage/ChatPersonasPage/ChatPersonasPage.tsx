import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { chatPersonas, deleteChatPersona } from "#/api/queries/chatAgents";
import type { ChatPersona } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { ChatPersonasPageView } from "#/modules/chatAgents/ChatPersonasPageView";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";

const ChatPersonasPage: FC = () => {
	const queryClient = useQueryClient();
	const { chat_agents: isChatAgentsEnabled } = useFeatureVisibility();
	const { organization, organizationPermissions } = useOrganizationSettings();
	const [personaToDelete, setPersonaToDelete] = useState<ChatPersona>();

	const personasQuery = useQuery(chatPersonas(organization?.id));
	const deleteMutation = useMutation(deleteChatPersona(queryClient));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<title>
				{pageTitle("Personas", organization.display_name || organization.name)}
			</title>

			<RequirePermission
				isFeatureVisible={organizationPermissions?.createChatPersonas ?? false}
			>
				<ChatPersonasPageView
					personas={personasQuery.data}
					error={personasQuery.error}
					canEdit={organizationPermissions?.createChatPersonas ?? false}
					isEntitled={isChatAgentsEnabled}
					organizationId={organization.id}
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
		</div>
	);
};

export default ChatPersonasPage;
