import { type FC, useState } from "react";
import { Link } from "react-router";
import type { ExternalAuthProviderEntry } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { AvatarData } from "#/components/Avatar/AvatarData";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
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
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Loader } from "#/components/Loader/Loader";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { LockIcon, MoreVerticalIcon, PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import { docs } from "#/utils/docs";

export type ExternalAuthSettingsPageViewProps = {
	providers: ExternalAuthProviderEntry[] | undefined;
	isLoading: boolean;
	error: Error | null;
	canCreateProvider: boolean;
	onDeleteProvider: (provider: ExternalAuthProviderEntry) => Promise<void>;
	deleteProviderLoading: boolean;
};

export const ExternalAuthSettingsPageView: FC<
	ExternalAuthSettingsPageViewProps
> = ({
	providers,
	isLoading,
	error,
	canCreateProvider,
	onDeleteProvider,
	deleteProviderLoading,
}) => {
	const [providerToDelete, setProviderToDelete] =
		useState<ExternalAuthProviderEntry | null>(null);

	return (
		<>
			<SettingsHeader
				actions={
					<>
						<SettingsHeaderDocsLink href={docs("/admin/external-auth")} />
						{canCreateProvider && (
							<Button asChild>
								<Link to="/deployment/external-auth/add">
									<PlusIcon aria-hidden="true" />
									Add provider
								</Link>
							</Button>
						)}
					</>
				}
			>
				<SettingsHeaderTitle>External Authentication</SettingsHeaderTitle>
				<SettingsHeaderDescription>
					Coder integrates with GitHub, GitLab, BitBucket, Azure Repos, and
					OpenID Connect to authenticate developers with external services.
				</SettingsHeaderDescription>
			</SettingsHeader>

			{error && <ErrorAlert error={error} />}

			{isLoading && <Loader />}

			{!isLoading && !error && (
				<Table>
					<TableHeader>
						<TableRow>
							<TableHead>Provider</TableHead>
							<TableHead>Type</TableHead>
							<TableHead>Provider ID</TableHead>
							<TableHead>Client ID</TableHead>
							<TableHead>Source</TableHead>
							<TableHead className="w-10" />
						</TableRow>
					</TableHeader>
					<TableBody>
						{(!providers || providers.length === 0) ? (
							<TableEmpty
								message="No external auth providers configured."
								cta={
									canCreateProvider ? (
										<Button asChild>
											<Link to="/deployment/external-auth/add">
												Add a provider
											</Link>
										</Button>
									) : undefined
								}
							/>
						) : (
							providers.map((provider) => {
								const isDatabaseSourced = provider.source === "database";
								return (
									<TableRow key={provider.id}>
										<TableCell>
											<Link
												to={`/deployment/external-auth/${provider.id}`}
												className="no-underline text-inherit hover:underline"
											>
												<AvatarData
													title={provider.display_name || provider.provider_id}
													subtitle={provider.regex || undefined}
													src={provider.display_icon || undefined}
												/>
											</Link>
										</TableCell>
										<TableCell>{provider.type || "custom"}</TableCell>
										<TableCell>
											<code>{provider.provider_id}</code>
										</TableCell>
										<TableCell>
											<code>{provider.client_id}</code>
										</TableCell>
										<TableCell>
											<SourceBadge source={provider.source} />
										</TableCell>
										<TableCell>
											{isDatabaseSourced && (
												<DropdownMenu>
													<DropdownMenuTrigger asChild>
														<Button
															variant="subtle"
															size="icon"
															aria-label="Open menu"
														>
															<MoreVerticalIcon className="size-4" />
														</Button>
													</DropdownMenuTrigger>
													<DropdownMenuContent align="end">
														<DropdownMenuItem asChild>
															<Link
																to={`/deployment/external-auth/${provider.id}`}
															>
																<PencilIcon aria-hidden="true" className="size-4" />
																Edit
															</Link>
														</DropdownMenuItem>
														<DropdownMenuItem
															className="text-destructive"
															onClick={() => setProviderToDelete(provider)}
														>
															<Trash2Icon className="size-4" />
															Delete
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
			)}

			<ConfirmDialog
				open={providerToDelete !== null}
				title="Delete external auth provider"
				description={`Are you sure you want to delete "${providerToDelete?.display_name || providerToDelete?.provider_id}"?`}
				confirmText="Delete"
				onConfirm={() => {
					if (providerToDelete) {
						onDeleteProvider(providerToDelete).then(() => {
							setProviderToDelete(null);
						});
					}
				}}
				onClose={() => setProviderToDelete(null)}
				confirmLoading={deleteProviderLoading}
			/>
		</>
	);
};

const SourceBadge: FC<{ source: string }> = ({ source }) => {
	if (source === "env") {
		return (
			<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-muted text-muted-foreground">
				<LockIcon aria-hidden="true" className="size-3.5" />
				Environment
			</span>
		);
	}
	return (
		<span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-muted text-muted-foreground">
			Database
		</span>
	);
};
