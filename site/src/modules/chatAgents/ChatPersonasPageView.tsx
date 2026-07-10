import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink, useNavigate } from "react-router";
import type { ChatPersona } from "#/api/typesGenerated";
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

interface ChatPersonasPageViewProps {
	personas: readonly ChatPersona[] | undefined;
	error: unknown;
	// Whether the current user may create/update/delete entries in
	// this page's scope. Ignored when the feature is not entitled.
	canEdit: boolean;
	isEntitled: boolean;
	// Set on organization pages; deployment and builtin entries render
	// read-only for reference.
	organizationId?: string;
	onDelete: (persona: ChatPersona) => void;
}

export const ChatPersonasPageView: FC<ChatPersonasPageViewProps> = ({
	personas,
	error,
	canEdit,
	isEntitled,
	organizationId,
	onDelete,
}) => {
	const navigate = useNavigate();
	const canWrite = canEdit && isEntitled;

	return (
		<div className="flex flex-col gap-6">
			{!isEntitled && (
				<PaywallPremium
					message="Chat personas"
					description="Create custom chat personas to give agents tailored system prompts and model preferences."
					documentationLink={docs("/ai-coder/agents")}
				/>
			)}
			<SettingsHeader
				actions={
					canWrite && (
						<Button variant="outline" asChild>
							<RouterLink to="create">
								<PlusIcon />
								Create persona
							</RouterLink>
						</Button>
					)
				}
			>
				<SettingsHeaderTitle>Personas</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Personas bundle a system prompt with a preferred model. Agents
					reference a persona to define how they behave. Builtin personas are
					read-only.
				</SettingsHeaderDescription>
			</SettingsHeader>
			{Boolean(error) && <ErrorAlert error={error} />}
			<Table aria-label="Personas">
				<TableHeader>
					<TableRow>
						<TableHead className="w-2/5">Name</TableHead>
						<TableHead className="w-1/5">Scope</TableHead>
						<TableHead className="w-1/5">Status</TableHead>
						<TableHead className="w-12">
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{personas === undefined ? (
						<TableLoader />
					) : personas.length === 0 ? (
						<TableEmpty
							message="No personas"
							description="Personas will appear here."
						/>
					) : (
						personas.map((persona) => {
							const editable =
								canWrite && isEditableInScope(persona, organizationId);
							return (
								<TableRow
									key={persona.id}
									data-testid={`persona-${persona.slug}`}
								>
									<TableCell>
										<div className="flex flex-col">
											<span className="text-content-primary">
												{persona.name || persona.slug}
											</span>
											<span className="text-xs text-content-secondary">
												{persona.slug}
											</span>
										</div>
									</TableCell>
									<TableCell>
										<ScopeBadge scope={chatCatalogScope(persona)} />
									</TableCell>
									<TableCell>
										<EnabledBadge enabled={persona.enabled} />
									</TableCell>
									<TableCell>
										{editable && (
											<DropdownMenu>
												<DropdownMenuTrigger asChild>
													<Button
														size="icon"
														variant="subtle"
														aria-label={`Open menu for persona ${persona.name || persona.slug}`}
													>
														<EllipsisVerticalIcon aria-hidden="true" />
													</Button>
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end">
													<DropdownMenuItem
														onClick={() => void navigate(persona.id)}
													>
														Edit
													</DropdownMenuItem>
													<DropdownMenuItem
														className="text-content-destructive focus:text-content-destructive"
														onClick={() => onDelete(persona)}
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
