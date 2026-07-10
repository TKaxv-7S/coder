import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	chatAgents,
	chatPersonas,
	deleteChatAgent,
} from "#/api/queries/chatAgents";
import type { ChatAgent } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { ChatAgentsPageView } from "#/modules/chatAgents/ChatAgentsPageView";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";

const ChatAgentsPage: FC = () => {
	const { permissions } = useAuthenticated();
	const queryClient = useQueryClient();
	const { chat_agents: isChatAgentsEnabled } = useFeatureVisibility();
	const [agentToDelete, setAgentToDelete] = useState<ChatAgent>();

	const agentsQuery = useQuery(chatAgents());
	const personasQuery = useQuery(chatPersonas());
	const deleteMutation = useMutation(deleteChatAgent(queryClient));

	return (
		<RequirePermission isFeatureVisible={permissions.editDeploymentConfig}>
			<title>{pageTitle("Agents", "AI Settings")}</title>

			<ChatAgentsPageView
				agents={agentsQuery.data}
				personas={personasQuery.data}
				error={agentsQuery.error ?? personasQuery.error}
				canEdit={permissions.editDeploymentConfig}
				isEntitled={isChatAgentsEnabled}
				onDelete={setAgentToDelete}
			/>

			<DeleteDialog
				key={agentToDelete?.id}
				isOpen={agentToDelete !== undefined}
				confirmLoading={deleteMutation.isPending}
				name={agentToDelete?.name ?? ""}
				entity="agent"
				onCancel={() => setAgentToDelete(undefined)}
				onConfirm={async () => {
					if (!agentToDelete) {
						return;
					}
					try {
						await deleteMutation.mutateAsync(agentToDelete.id);
						toast.success(`Agent "${agentToDelete.name}" deleted.`);
						setAgentToDelete(undefined);
					} catch (error) {
						toast.error(getErrorMessage(error, "Failed to delete agent."), {
							description: getErrorDetail(error),
						});
					}
				}}
			/>
		</RequirePermission>
	);
};

export default ChatAgentsPage;
