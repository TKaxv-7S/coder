import type { FC } from "react";
import { Link } from "react-router";
import { Button } from "#/components/Button/Button";

interface SummaryStepProps {
	agentsEnabled: boolean;
	providerConfigured: boolean;
	onComplete: () => void;
}

export const SummaryStep: FC<SummaryStepProps> = ({
	agentsEnabled,
	providerConfigured,
	onComplete,
}) => {
	return (
		<div className="flex flex-col gap-8">
			<header className="text-center">
				<div className="text-4xl mb-4">🎉</div>
				<h2 className="text-2xl font-semibold m-0">You're all set!</h2>
				<p className="text-sm text-content-secondary mt-2 mb-0">
					Here's a summary of what was configured.
				</p>
			</header>

			{/* Configuration summary */}
			<div className="flex flex-col gap-3">
				<div className="flex items-center gap-3 p-3 rounded-md bg-surface-secondary">
					<span className="text-sm">
						{agentsEnabled ? "✅" : "➖"} AI Agents{" "}
						{agentsEnabled ? "enabled" : "not enabled"}
					</span>
				</div>
				{agentsEnabled && (
					<div className="flex items-center gap-3 p-3 rounded-md bg-surface-secondary">
						<span className="text-sm">
							{providerConfigured ? "✅" : "➖"} AI Provider{" "}
							{providerConfigured ? "configured" : "not configured"}
						</span>
					</div>
				)}
			</div>

			{/* Quick links */}
			<div className="flex flex-col gap-2">
				<h3 className="text-sm font-semibold m-0">Get started</h3>
				<div className="grid grid-cols-2 gap-3">
					<QuickLink to="/templates" label="Templates" desc="Browse and create templates" />
					<QuickLink to="/workspaces" label="Workspaces" desc="Launch a workspace" />
					<QuickLink to="/agents" label="Agents" desc="Manage AI agents" />
					<QuickLink
						to="https://coder.com/docs"
						label="Documentation"
						desc="Read the docs"
						external
					/>
				</div>
			</div>

			<div className="flex justify-center pt-2">
				<Button onClick={onComplete} size="lg">
					Go to Dashboard
				</Button>
			</div>
		</div>
	);
};

const QuickLink: FC<{
	to: string;
	label: string;
	desc: string;
	external?: boolean;
}> = ({ to, label, desc, external }) => {
	const className =
		"flex flex-col gap-0.5 p-3 rounded-md border border-border hover:border-border-hover transition-colors no-underline";

	if (external) {
		return (
			<a
				href={to}
				target="_blank"
				rel="noreferrer"
				className={className}
			>
				<span className="text-sm font-medium text-content-primary">
					{label}
				</span>
				<span className="text-xs text-content-secondary">{desc}</span>
			</a>
		);
	}

	return (
		<Link to={to} className={className}>
			<span className="text-sm font-medium text-content-primary">
				{label}
			</span>
			<span className="text-xs text-content-secondary">{desc}</span>
		</Link>
	);
};
