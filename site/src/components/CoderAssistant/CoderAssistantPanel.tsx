import {
	EllipsisVerticalIcon,
	ExternalLinkIcon,
	PlusIcon,
	SparklesIcon,
	XIcon,
} from "lucide-react";
import { type FC, useEffect, useRef } from "react";
import { useNavigate } from "react-router";
import { Button } from "#/components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import {
	AgentChatInput,
	type ChatMessageInputRef,
} from "#/pages/AgentsPage/components/AgentChatInput";
import {
	type ChatStore,
	selectOrderedMessageIDs,
	useChatSelector,
} from "#/pages/AgentsPage/components/ChatConversation/chatStore";
import type { ModelSelectorOption } from "#/pages/AgentsPage/components/ChatElements";
import { ChatPageTimeline } from "#/pages/AgentsPage/components/ChatPageContent";
import { buildAgentChatPath } from "#/pages/AgentsPage/utils/navigation";
import type { ChatDetailError } from "#/pages/AgentsPage/utils/usageLimitMessage";
import { cn } from "#/utils/cn";

const SUGGESTED_PROMPTS = [
	"What can you do?",
	"Create a workspace for me",
	"Help me build a template",
];

interface CoderAssistantPanelProps {
	open: boolean;
	onClose: () => void;
	onNewChat: () => void;
	onDisable: () => void;
	onSendMessage: (text: string) => void;
	isThinking: boolean;
	isSendPending: boolean;
	isStreaming: boolean;
	onInterrupt: () => void;
	isInterruptPending: boolean;
	chatId: string | null;
	store: ChatStore;
	persistedError: ChatDetailError | undefined;
	// Model selector state, provided by CoderAssistantProvider.
	modelOptions: readonly ModelSelectorOption[];
	selectedModel: string;
	onModelChange: (id: string) => void;
	hasModelOptions: boolean;
	modelSelectorPlaceholder: string;
	isModelCatalogLoading: boolean;
}

export const CoderAssistantPanel: FC<CoderAssistantPanelProps> = ({
	open,
	onClose,
	onNewChat,
	onDisable,
	onSendMessage,
	isThinking,
	isSendPending,
	isStreaming,
	onInterrupt,
	isInterruptPending,
	chatId,
	store,
	persistedError,
	modelOptions,
	selectedModel,
	onModelChange,
	hasModelOptions,
	modelSelectorPlaceholder,
	isModelCatalogLoading,
}) => {
	const messagesEndRef = useRef<HTMLDivElement>(null);
	const inputRef = useRef<ChatMessageInputRef>(null);
	const navigate = useNavigate();

	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const messageCount = orderedMessageIDs.length;

	// Auto-scroll to bottom on new messages or thinking state change.
	// biome-ignore lint/correctness/useExhaustiveDependencies: messageCount and isThinking are intentional scroll triggers
	useEffect(() => {
		messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
	}, [messageCount, isThinking]);

	// Focus input when panel opens.
	useEffect(() => {
		if (open) {
			inputRef.current?.focus();
		}
	}, [open]);

	// Escape closes the panel. Radix layers (dropdowns, selects) call
	// preventDefault when they consume Escape, so checking
	// defaultPrevented keeps this from firing while a menu is open.
	useEffect(() => {
		if (!open) {
			return;
		}
		const handleKeyDown = (event: KeyboardEvent) => {
			if (event.key === "Escape" && !event.defaultPrevented) {
				onClose();
			}
		};
		document.addEventListener("keydown", handleKeyDown);
		return () => document.removeEventListener("keydown", handleKeyDown);
	}, [open, onClose]);

	const handleSend = (text: string) => {
		onSendMessage(text);
		inputRef.current?.clear();
	};

	if (!open) return null;

	return (
		<div
			className={cn(
				"fixed bottom-[7rem] right-6 z-50",
				"w-[420px] h-[640px] max-h-[80vh]",
				"flex flex-col",
				"rounded-xl shadow-2xl",
				"border border-border border-solid",
				"bg-surface-primary",
				"animate-in slide-in-from-bottom-2 fade-in duration-200",
			)}
		>
			{/* Header. Mirrors the agents chat top bar (ChatTopBar). */}
			<div className="flex shrink-0 items-center gap-2 px-4 py-1.5">
				<div className="flex min-w-0 flex-1 items-center gap-1">
					<span className="truncate text-sm font-medium text-content-primary">
						Coder Assistant
					</span>
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="subtle"
								size="icon"
								className="size-7 text-content-secondary hover:text-content-primary"
								aria-label="Assistant settings"
							>
								<EllipsisVerticalIcon className="size-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="start">
							<DropdownMenuItem onClick={onNewChat}>
								<PlusIcon className="size-4" />
								New chat
							</DropdownMenuItem>
							{chatId && (
								<DropdownMenuItem
									onClick={() => {
										onClose();
										void navigate(buildAgentChatPath({ chatId }));
									}}
								>
									<ExternalLinkIcon className="size-4" />
									Open in Agents
								</DropdownMenuItem>
							)}
							<DropdownMenuItem
								onClick={() => {
									onClose();
									void navigate("/ai/settings/providers");
								}}
							>
								AI settings
							</DropdownMenuItem>
							<DropdownMenuSeparator />
							<DropdownMenuItem onClick={onDisable}>
								Disable assistant
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>
				<div className="flex items-center gap-1">
					<Button
						variant="subtle"
						size="icon"
						onClick={onClose}
						className="size-7 text-content-secondary hover:text-content-primary"
						aria-label="Close Coder Assistant"
					>
						<XIcon className="size-4" />
					</Button>
				</div>
			</div>

			{/* Messages. Same scroller treatment as the agents chat view. */}
			<div className="min-h-0 flex-1 overflow-y-auto [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
				<div className="px-4">
					{chatId ? (
						<>
							{/* ChatPageTimeline applies mx-auto/max-w internally;
							    at 420px wide it just fills the panel. Its py-6 is
							    tighter here via a negative top margin. */}
							<div className="-mt-2">
								<ChatPageTimeline
									store={store}
									persistedError={persistedError}
								/>
							</div>
							<div ref={messagesEndRef} />
						</>
					) : (
						<div className="flex h-full min-h-[400px] flex-col items-center justify-center gap-3 text-center">
							{isThinking ? (
								<span className="inline-flex items-center gap-1 text-sm text-content-secondary">
									<span className="animate-bounce [animation-delay:0ms]">
										.
									</span>
									<span className="animate-bounce [animation-delay:150ms]">
										.
									</span>
									<span className="animate-bounce [animation-delay:300ms]">
										.
									</span>
								</span>
							) : (
								<>
									<SparklesIcon className="size-8 text-content-disabled" />
									<div>
										<p className="text-sm font-medium text-content-primary">
											Hi, I'm your Coder Assistant
										</p>
										<p className="mt-1 text-xs text-content-secondary">
											Your AI assistant for Coder. Ask me anything about your
											workspaces, templates, or development environment.
										</p>
									</div>
									<div className="flex flex-wrap justify-center gap-2">
										{SUGGESTED_PROMPTS.map((prompt) => (
											<button
												key={prompt}
												type="button"
												disabled={!hasModelOptions}
												onClick={() => onSendMessage(prompt)}
												className={cn(
													"cursor-pointer rounded-full border border-solid border-border",
													"bg-surface-secondary px-3 py-1.5 text-xs text-content-secondary",
													"hover:bg-surface-tertiary hover:text-content-primary",
													"disabled:cursor-not-allowed disabled:opacity-50",
												)}
											>
												{prompt}
											</button>
										))}
									</div>
								</>
							)}
							{persistedError && (
								<p className="text-xs text-content-destructive">
									{persistedError.message}
								</p>
							)}
						</div>
					)}
				</div>
			</div>

			{/* Composer. The real agents chat input, minimally wired.
			    AgentChatInput adds its own bottom padding (pb-4 at sm+). */}
			<div className="shrink-0 px-4">
				<AgentChatInput
					inputRef={inputRef}
					onSend={handleSend}
					placeholder="Type a message..."
					isDisabled={!hasModelOptions}
					isLoading={isSendPending}
					isStreaming={isStreaming}
					onInterrupt={onInterrupt}
					isInterruptPending={isInterruptPending}
					selectedModel={selectedModel}
					onModelChange={onModelChange}
					modelOptions={modelOptions}
					modelSelectorPlaceholder={modelSelectorPlaceholder}
					hasModelOptions={hasModelOptions}
					isModelCatalogLoading={isModelCatalogLoading}
					canConfigureAgentSetup={false}
				/>
			</div>
		</div>
	);
};
