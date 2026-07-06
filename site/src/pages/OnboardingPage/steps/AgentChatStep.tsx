import type { FC } from "react";
import { Button } from "#/components/Button/Button";

interface AgentChatStepProps {
	onFinish: () => void;
}

const BLINK_ENABLED_KEY = "blink_enabled";

export const AgentChatStep: FC<AgentChatStepProps> = ({ onFinish }) => {
	const enableBlink = () => {
		localStorage.setItem(BLINK_ENABLED_KEY, "true");
		onFinish();
	};

	return (
		<div className="flex flex-col gap-6 h-full items-center justify-center">
			<header className="text-center max-w-lg">
				<h2 className="text-2xl font-semibold m-0">Meet Blink</h2>
				<p className="text-sm text-content-secondary mt-2 mb-0">
					Your built-in Coder assistant
				</p>
			</header>

			{/* Blink button mockup */}
			<div className="relative w-72 h-48 rounded-lg border border-border bg-surface-secondary flex items-end justify-end p-4">
				<div className="absolute top-4 left-4 text-xs text-content-secondary">
					Coder Dashboard
				</div>
				<div className="w-12 h-12 rounded-full bg-content-link flex items-center justify-center text-white text-lg font-bold shadow-lg">
					B
				</div>
			</div>

			<div className="text-center max-w-md space-y-2">
				<p className="text-sm text-content-primary m-0">
					Blink lives in the bottom-right corner of your dashboard.
				</p>
				<p className="text-sm text-content-secondary m-0">
					It can help you manage templates, create workspaces, troubleshoot
					issues, and answer questions about your Coder deployment.
				</p>
			</div>

			<div className="flex gap-3 pt-2">
				<Button variant="outline" onClick={onFinish}>
					Skip
				</Button>
				<Button onClick={enableBlink}>Enable Blink</Button>
			</div>
		</div>
	);
};
