import type { FC } from "react";
import { Button } from "#/components/Button/Button";

interface AgentChatStepProps {
	onFinish: () => void;
}

export const AgentChatStep: FC<AgentChatStepProps> = ({ onFinish }) => {
	// TODO: Wire up real chat via createChat mutation, useChatStore, and
	// watchChat for WebSocket streaming. Render messages using patterns
	// from ChatConversation/ConversationTimeline.tsx.

	return (
		<div className="flex flex-col gap-4 h-full">
			<header>
				<h2 className="text-2xl font-semibold m-0">Agent Chat</h2>
				<p className="text-sm text-content-secondary mt-2 mb-0">
					Chat with the Coder agent to help set up your environment.
				</p>
			</header>

			{/* Chat container placeholder */}
			<div className="flex-1 min-h-[400px] rounded-lg border border-border bg-surface-secondary flex flex-col">
				{/* Messages area */}
				<div className="flex-1 flex items-center justify-center p-8">
					<div className="text-center text-content-secondary">
						<div className="text-4xl mb-4">💬</div>
						<p className="text-sm font-medium m-0">
							Agent chat will appear here
						</p>
						<p className="text-xs mt-1 mb-0">
							This will be an interactive chat powered by your configured AI
							provider.
						</p>
					</div>
				</div>

				{/* Input area placeholder */}
				<div className="border-t border-border p-4">
					<div className="flex gap-2">
						<input
							type="text"
							disabled
							placeholder="Ask the agent anything about Coder..."
							className="flex-1 h-10 rounded-md border border-border bg-transparent px-3 text-sm
								placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-50"
						/>
						<button
							type="button"
							disabled
							className="h-10 px-4 rounded-md bg-surface-tertiary text-content-secondary text-sm
								cursor-not-allowed opacity-50"
						>
							Send
						</button>
					</div>
				</div>
			</div>

			<div className="flex justify-end pt-2">
				<Button onClick={onFinish}>Finish Setup</Button>
			</div>
		</div>
	);
};
