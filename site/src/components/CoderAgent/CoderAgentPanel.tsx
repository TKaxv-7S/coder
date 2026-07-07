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
				"fixed bottom-20 right-6 z-50",
				"w-[400px] h-[600px] max-h-[80vh]",
				"flex flex-col",
				"rounded-xl shadow-2xl",
				"border border-border border-solid",
				"bg-surface-primary",
				"animate-in slide-in-from-bottom-2 fade-in duration-200",
			)}
		>
			{/* Header */}
			<div
				className={cn(
					"flex items-center justify-between",
					"px-4 py-3",
					"border-b border-border border-solid",
				)}
			>
				<div className="flex items-center gap-2">
					<SparklesIcon className="size-4 text-content-link" />
					<h2 className="text-sm font-semibold text-content-primary">
						Coder Agent
					</h2>
				</div>
				<div className="flex items-center gap-1">
					<button
						type="button"
						onClick={onNewChat}
						aria-label="New chat"
						className={cn(
							"p-1.5 rounded-md",
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
							"p-1.5 rounded-md",
							"text-content-secondary hover:text-content-primary",
							"hover:bg-surface-secondary",
							"transition-colors",
						)}
					>
						<XIcon className="size-4" />
					</button>
				</div>
			</div>

			{/* Messages */}
			<div className="flex-1 overflow-y-auto px-4">
				{chatId ? (
					<>
						<ChatPageTimeline store={store} persistedError={persistedError} />
						<div ref={messagesEndRef} />
					</>
				) : (
					<div className="flex flex-col items-center justify-center h-full text-center gap-3 py-3">
						{isThinking ? (
							<span className="inline-flex items-center gap-1 text-sm text-content-secondary">
								<span className="animate-bounce [animation-delay:0ms]">.</span>
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
									<p className="text-xs text-content-secondary mt-1">
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

			{/* Footer / Input */}
			<div className={cn("px-4 py-3", "border-t border-border border-solid")}>
				<div className="flex items-center gap-2">
					<input
						ref={inputRef}
						type="text"
						value={inputValue}
						onChange={(e) => setInputValue(e.target.value)}
						onKeyDown={handleKeyDown}
						placeholder="Ask Coder Agent..."
						aria-label="Message Coder Agent"
						className={cn(
							"flex-1 min-w-0",
							"px-3 py-2 text-sm",
							"rounded-lg border border-border border-solid",
							"bg-surface-primary text-content-primary",
							"placeholder:text-content-disabled",
							"focus:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
						)}
					/>
					<button
						type="button"
						onClick={handleSend}
						disabled={!inputValue.trim()}
						aria-label="Send message"
						className={cn(
							"flex items-center justify-center",
							"w-8 h-8 rounded-lg",
							"bg-surface-invert-primary text-surface-primary",
							"hover:opacity-90 transition-opacity",
							"disabled:opacity-40 disabled:cursor-not-allowed",
						)}
					>
						<ArrowUpIcon className="size-4" />
					</button>
				</div>
			</div>
		</div>
	);
};
