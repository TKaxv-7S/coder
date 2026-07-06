import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { ProductLogo } from "#/components/Icons/ProductLogo";
import { Switch } from "#/components/Switch/Switch";

interface WelcomeStepProps {
	agentsEnabled: boolean;
	onAgentsEnabledChange: (enabled: boolean) => void;
	onContinue: () => void;
}

export const WelcomeStep: FC<WelcomeStepProps> = ({
	agentsEnabled,
	onAgentsEnabledChange,
	onContinue,
}) => {
	return (
		<div className="flex flex-col gap-8">
			<header className="flex flex-col items-center text-center gap-4">
				<ProductLogo className="h-10" />
				<h1 className="text-3xl font-semibold m-0">Welcome to Coder!</h1>
				<p className="text-sm text-content-secondary m-0 max-w-sm leading-relaxed">
					Coder provides self-hosted, secure development environments. Let's
					get your instance configured so your team can start building.
				</p>
			</header>

			<label
				htmlFor="agents-toggle"
				className="flex items-start gap-4 p-4 rounded-lg border border-border cursor-pointer hover:border-border-hover transition-colors"
			>
				<Switch
					id="agents-toggle"
					checked={agentsEnabled}
					onCheckedChange={(checked) =>
						onAgentsEnabledChange(checked === true)
					}
					className="mt-0.5"
				/>
				<div className="flex flex-col gap-1">
					<span className="text-sm font-semibold">
						Enable AI Agents to help with setup
					</span>
					<span className="text-xs text-content-secondary leading-relaxed">
						An AI assistant will help you set up templates, create workspaces,
						and answer questions about Coder. You can configure an AI provider
						in the next step.
					</span>
				</div>
			</label>

			<div className="flex justify-end">
				<Button onClick={onContinue}>
					Continue
				</Button>
			</div>
		</div>
	);
};
