import type { FC } from "react";
import { Badge } from "#/components/Badge/Badge";

interface ChatCatalogEntry {
	readonly builtin: boolean;
	readonly organization_id?: string;
}

type ChatCatalogScope = "builtin" | "deployment" | "organization";

export const chatCatalogScope = (entry: ChatCatalogEntry): ChatCatalogScope => {
	if (entry.builtin) {
		return "builtin";
	}
	return entry.organization_id ? "organization" : "deployment";
};

// An entry is editable only when it is not builtin and belongs to the
// scope the current page manages: the organization when organizationId
// is set, the deployment otherwise.
export const isEditableInScope = (
	entry: ChatCatalogEntry,
	organizationId?: string,
): boolean => {
	if (entry.builtin) {
		return false;
	}
	return organizationId
		? entry.organization_id === organizationId
		: !entry.organization_id;
};

const scopeLabels: Record<ChatCatalogScope, string> = {
	builtin: "Builtin",
	deployment: "Deployment",
	organization: "Organization",
};

export const ScopeBadge: FC<{ scope: ChatCatalogScope }> = ({ scope }) => (
	<Badge
		size="sm"
		variant={
			scope === "builtin"
				? "purple"
				: scope === "organization"
					? "green"
					: "default"
		}
	>
		{scopeLabels[scope]}
	</Badge>
);

export const EnabledBadge: FC<{ enabled: boolean }> = ({ enabled }) => (
	<Badge size="sm" variant={enabled ? "green" : "default"}>
		{enabled ? "Enabled" : "Disabled"}
	</Badge>
);
