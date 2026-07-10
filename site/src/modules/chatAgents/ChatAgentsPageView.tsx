import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { ChatAgent, ChatPersona } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { PaywallPremium } from "#/components/Paywall/PaywallPremium";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { TableLoader } from "#/components/TableLoader/TableLoader";
import { docs } from "#/utils/docs";
import {
	chatCatalogScope,
	EnabledBadge,
	isEditableInScope,
	ScopeBadge,
} from "./ScopeBadge";

interface ChatAgentsPageViewProps {
	agents: readonly ChatAgent[] | undefined;
	// Used to resolve each agent's persona name.
	personas: readonly ChatPersona[] | undefined;
	error: unknown;
	canEdit: boolean;
	isEntitled: boolean;
	organizationId?: string;
	onDelete: (agent: ChatAgent) => void;
}

export const ChatAgentsPageView: FC<ChatAgentsPageViewProps> = ({
	agents,
	personas,
	error,
	canEdit,
	isEntitled,
	organizationId,
	onDelete,
}) => {
	const navigate = useNavigate();
	const canWrite = canEdit && isEntitled;
	const personaName = (personaId: string): string => {
		const persona = personas?.find((p) => p.id === personaId);
		return persona ? persona.name || persona.slug : "Unknown persona";
	};

	return (
		<div className="flex flex-col gap-6">
			{!isEntitled && (
				<PaywallPremium
					message="Chat agents"
					description="Create named chat agents that run a persona with an optional prompt append and model override."
					documentationLink={docs("/ai-coder/agents")}
				/>
			)}
			<SettingsHeader
				actions={
					canWrite && (
						<Button variant="outline" asChild>
							<RouterLink to="create">
								<PlusIcon />
								Create agent
							</RouterLink>
						</Button>
					)
				}
			>
				<SettingsHeaderTitle>Agents</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Agents are named entries users can pick when creating a chat. Each
					agent runs a persona and can append to its prompt or override its
					model. Builtin agents are read-only.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{Boolean(error) && <ErrorAlert error={error} />}
			<Table aria-label="Agents">
				<TableHeader>
					<TableRow>
						<TableHead className="w-1/3">Name</TableHead>
						<TableHead className="w-1/4">Persona</TableHead>
						<TableHead className="w-1/5">Scope</TableHead>
						<TableHead className="w-1/5">Status</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{agents === undefined ? (
						<TableLoader />
					) : agents.length === 0 ? (
						<TableEmpty
							message="No agents"
							description="Agents will appear here."
						/>
					) : (
						agents.map((agent) => {
							const editable =
								canWrite && isEditableInScope(agent, organizationId);
							return (
								<TableRow key={agent.id} data-testid={`agent-${agent.slug}`}>
									<TableCell>
										<div className="flex flex-col">
											<span className="text-content-primary">
												{agent.name || agent.slug}
											</span>
											<span className="text-xs text-content-secondary">
												{agent.slug}
											</span>
										</div>
									</TableCell>
									<TableCell>{personaName(agent.persona_id)}</TableCell>
									<TableCell>
										<ScopeBadge scope={chatCatalogScope(agent)} />
									</TableCell>
									<TableCell>
										<EnabledBadge enabled={agent.enabled} />
									</TableCell>
									<TableCell>
										{editable && (
											<DropdownMenu>
												<DropdownMenuTrigger asChild>
													<Button
														size="icon"
														variant="subtle"
														aria-label={`Open menu for agent ${agent.name || agent.slug}`}
													>
														<EllipsisVerticalIcon aria-hidden="true" />
													</Button>
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end">
													<DropdownMenuItem
														onClick={() => void navigate(agent.id)}
													>
														Edit
													</DropdownMenuItem>
													<DropdownMenuItem
														className="text-content-destructive focus:text-content-destructive"
														onClick={() => onDelete(agent)}
													>
														Delete&hellip;
													</DropdownMenuItem>
												</DropdownMenuContent>
											</DropdownMenu>
										)}
									</TableCell>
								</TableRow>
							);
						})
					)}
				</TableBody>
			</Table>
		</div>
	);
};
