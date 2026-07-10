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
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { ChatAgentsPageView } from "#/modules/chatAgents/ChatAgentsPageView";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";

const ChatAgentsPage: FC = () => {
	const queryClient = useQueryClient();
	const { chat_agents: isChatAgentsEnabled } = useFeatureVisibility();
	const { organization, organizationPermissions } = useOrganizationSettings();
	const [agentToDelete, setAgentToDelete] = useState<ChatAgent>();

	const agentsQuery = useQuery(chatAgents(organization?.id));
	const personasQuery = useQuery(chatPersonas(organization?.id));
	const deleteMutation = useMutation(deleteChatAgent(queryClient));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<title>
				{pageTitle("Agents", organization.display_name || organization.name)}
			</title>

			<RequirePermission
				isFeatureVisible={organizationPermissions?.createChatAgents ?? false}
			>
				<ChatAgentsPageView
					agents={agentsQuery.data}
					personas={personasQuery.data}
					error={agentsQuery.error ?? personasQuery.error}
					canEdit={organizationPermissions?.createChatAgents ?? false}
					isEntitled={isChatAgentsEnabled}
					organizationId={organization.id}
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
		</div>
	);
};

export default ChatAgentsPage;
