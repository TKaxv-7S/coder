import { ArrowUpIcon, PlusIcon, SparklesIcon, XIcon } from "lucide-react";
import {
	type FC,
	type KeyboardEvent,
	useEffect,
	useRef,
	useState,
} from "react";
import {
	type ChatStore,
	selectOrderedMessageIDs,
	useChatSelector,
} from "#/pages/AgentsPage/components/ChatConversation/chatStore";
import { ChatPageTimeline } from "#/pages/AgentsPage/components/ChatPageContent";
import type { ChatDetailError } from "#/pages/AgentsPage/utils/usageLimitMessage";
import { cn } from "#/utils/cn";

interface CoderAgentPanelProps {
	open: boolean;
	onClose: () => void;
	onNewChat: () => void;
	onSendMessage: (text: string) => void;
	isThinking: boolean;
	chatId: string | null;
	store: ChatStore;
	persistedError: ChatDetailError | undefined;
}

export const CoderAgentPanel: FC<CoderAgentPanelProps> = ({
	open,
	onClose,
	onNewChat,
	onSendMessage,
	isThinking,
	chatId,
	store,
	persistedError,
}) => {
	const [inputValue, setInputValue] = useState("");
	const messagesEndRef = useRef<HTMLDivElement>(null);
	const inputRef = useRef<HTMLInputElement>(null);

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

	const handleSend = () => {
		const text = inputValue.trim();
		if (!text) return;
		onSendMessage(text);
		setInputValue("");
	};

	const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			handleSend();
		}
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
			{/* Header. Flat and borderless to match the agents chat top bar. */}
			<div className="flex shrink-0 items-center justify-between gap-2 px-4 py-2">
				<div className="flex min-w-0 items-center gap-1.5">
					<SparklesIcon className="size-3.5 shrink-0 text-content-secondary" />
					<h2 className="truncate text-sm font-semibold text-content-primary">
						Coder Agent
					</h2>
				</div>
				<div className="flex items-center gap-1">
					<button
						type="button"
						onClick={onNewChat}
						aria-label="New chat"
						className={cn(
							"inline-flex size-7 items-center justify-center rounded-md",
							"border-0 bg-transparent cursor-pointer",
							"text-content-secondary hover:text-content-primary",
							"hover:bg-surface-secondary",
							"transition-colors",
						)}
					>
						<PlusIcon className="size-4" />
					</button>
					<button
						type="button"
						onClick={onClose}
						aria-label="Close Coder Agent"
						className={cn(
							"inline-flex size-7 items-center justify-center rounded-md",
							"border-0 bg-transparent cursor-pointer",
							"text-content-secondary hover:text-content-primary",
							"hover:bg-surface-secondary",
							"transition-colors",
						)}
					>
						<XIcon className="size-4" />
					</button>
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
											Hi, I'm your Coder Agent
										</p>
										<p className="mt-1 text-xs text-content-secondary">
											Your AI assistant for Coder. Ask me anything about your
											workspaces, templates, or development environment.
										</p>
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

			{/* Input. Mirrors the AgentChatInput composer container. */}
			<div className="shrink-0 px-4 pb-4 pt-2">
				<div
					className={cn(
						"relative flex items-end gap-2 rounded-xl p-2",
						"border border-border-default/80 border-solid",
						"bg-surface-secondary shadow-sm",
						"has-[input:focus]:ring-2 has-[input:focus]:ring-content-link/40",
					)}
				>
					<input
						ref={inputRef}
						type="text"
						value={inputValue}
						onChange={(e) => setInputValue(e.target.value)}
						onKeyDown={handleKeyDown}
						placeholder="Type a message..."
						aria-label="Message Coder Agent"
						className={cn(
							"min-w-0 flex-1 self-center",
							"border-0 bg-transparent px-2 py-1.5 text-sm",
							"text-content-primary placeholder:text-content-secondary",
							"focus:outline-none",
						)}
					/>
					<button
						type="button"
						onClick={handleSend}
						disabled={!inputValue.trim()}
						aria-label="Send message"
						className={cn(
							"inline-flex size-7 shrink-0 cursor-pointer items-center justify-center",
							"rounded-full border-0",
							"bg-surface-invert-primary text-surface-primary",
							"transition-opacity hover:opacity-90",
							"disabled:cursor-not-allowed disabled:opacity-40",
						)}
					>
						<ArrowUpIcon className="size-4" />
					</button>
				</div>
			</div>
		</div>
	);
};
