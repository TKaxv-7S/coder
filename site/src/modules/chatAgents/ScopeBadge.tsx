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

const scopeBadgeProps: Record<
	ChatCatalogScope,
	{ label: string; variant: "purple" | "green" | "default" }
> = {
	builtin: { label: "Builtin", variant: "purple" },
	deployment: { label: "Deployment", variant: "default" },
	organization: { label: "Organization", variant: "green" },
};

export const ScopeBadge: FC<{ scope: ChatCatalogScope }> = ({ scope }) => (
	<Badge size="sm" variant={scopeBadgeProps[scope].variant}>
		{scopeBadgeProps[scope].label}
	</Badge>
);

// EnabledStatusBadge is named to avoid colliding with the shared
// EnabledBadge in components/Badges, which has different styling.
export const EnabledStatusBadge: FC<{ enabled: boolean }> = ({ enabled }) => (
	<Badge size="sm" variant={enabled ? "green" : "default"}>
		{enabled ? "Enabled" : "Disabled"}
	</Badge>
);
