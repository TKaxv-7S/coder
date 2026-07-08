import { SparklesIcon } from "lucide-react";
import type { FC } from "react";
import { cn } from "#/utils/cn";

interface CoderAssistantButtonProps {
	open: boolean;
	onToggle: () => void;
	isThinking?: boolean;
	hasUnread?: boolean;
}

export const CoderAssistantButton: FC<CoderAssistantButtonProps> = ({
	open,
	onToggle,
	isThinking = false,
	hasUnread = false,
}) => {
	return (
		<button
			type="button"
			onClick={onToggle}
			aria-label={open ? "Close Coder Assistant" : "Open Coder Assistant"}
			aria-expanded={open}
			className={cn(
				"fixed bottom-12 right-6 z-50",
				"flex items-center justify-center",
				"w-12 h-12 rounded-full",
				"bg-surface-secondary text-content-primary",
				"border border-solid border-border hover:border-border-secondary",
				"shadow-lg hover:scale-105 transition-transform",
				"focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-content-link",
			)}
		>
			<SparklesIcon className="size-5" />

			{/* Unread indicator */}
			{hasUnread && !open && !isThinking && (
				<span
					className={cn(
						"absolute -top-0.5 -right-0.5",
						"w-3 h-3 rounded-full",
						"bg-content-link",
						"animate-pulse",
					)}
				/>
			)}

			{/* Thinking indicator */}
			{isThinking && (
				<>
					<span
						className={cn(
							"absolute -top-0.5 -right-0.5",
							"w-3 h-3 rounded-full",
							"bg-content-link",
							"animate-ping",
						)}
					/>
					<span
						className={cn(
							"absolute -top-0.5 -right-0.5",
							"w-3 h-3 rounded-full",
							"bg-content-link",
						)}
					/>
				</>
			)}
		</button>
	);
};
