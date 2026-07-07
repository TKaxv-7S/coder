import {
	createContext,
	type FC,
	type PropsWithChildren,
	useCallback,
	useContext,
	useEffect,
	useRef,
	useState,
} from "react";
import type { CoderAgentMessage } from "./CoderAgentPanel";

interface CoderAgentContextValue {
	enabled: boolean;
	open: boolean;
	toggle: () => void;
	close: () => void;
	messages: CoderAgentMessage[];
	sendMessage: (text: string) => void;
	startNewChat: () => void;
	isThinking: boolean;
	chatId: string | null;
}

const CoderAgentContext = createContext<CoderAgentContextValue | null>(null);

function readLocalStorage(key: string, fallback: string): string {
	try {
		return localStorage.getItem(key) ?? fallback;
	} catch {
		return fallback;
	}
}

function writeLocalStorage(key: string, value: string): void {
	try {
		localStorage.setItem(key, value);
	} catch {
		// Storage may be unavailable in some contexts.
	}
}

export const CoderAgentProvider: FC<
	PropsWithChildren<{ forceEnabled?: boolean }>
> = ({ children, forceEnabled }) => {
	const [enabled] = useState(
		() =>
			forceEnabled ||
			readLocalStorage("coder_agent_enabled", "false") === "true",
	);
	const [open, setOpen] = useState(false);
	const [messages, setMessages] = useState<CoderAgentMessage[]>([]);
	const [isThinking, setIsThinking] = useState(false);
	const [chatId, setChatId] = useState<string | null>(
		() => readLocalStorage("coder_agent_chat_id", "") || null,
	);

	// Track pending timeout so we can cancel it on unmount or new chat.
	const pendingTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);

	useEffect(() => {
		return () => {
			if (pendingTimeout.current !== null) {
				clearTimeout(pendingTimeout.current);
			}
		};
	}, []);

	const toggle = useCallback(() => {
		setOpen((prev) => !prev);
	}, []);

	const close = useCallback(() => {
		setOpen(false);
	}, []);

	const sendMessage = useCallback((text: string) => {
		const userMessage: CoderAgentMessage = {
			id: crypto.randomUUID(),
			role: "user",
			content: text,
			timestamp: new Date(),
		};

		setMessages((prev) => [...prev, userMessage]);
		setIsThinking(true);

		// Cancel any previously pending response.
		if (pendingTimeout.current !== null) {
			clearTimeout(pendingTimeout.current);
		}

		// Stub: simulate assistant response after a short delay.
		// This will be wired to createChatMessage later.
		pendingTimeout.current = setTimeout(() => {
			pendingTimeout.current = null;
			const assistantMessage: CoderAgentMessage = {
				id: crypto.randomUUID(),
				role: "assistant",
				content:
					"This is a prototype response. The Coder Agent will be connected to the AI backend soon.",
				timestamp: new Date(),
			};
			setMessages((prev) => [...prev, assistantMessage]);
			setIsThinking(false);
		}, 1500);
	}, []);

	const startNewChat = useCallback(() => {
		// Cancel any pending stub response.
		if (pendingTimeout.current !== null) {
			clearTimeout(pendingTimeout.current);
			pendingTimeout.current = null;
		}
		setMessages([]);
		setIsThinking(false);
		const newId = crypto.randomUUID();
		setChatId(newId);
		writeLocalStorage("coder_agent_chat_id", newId);
	}, []);

	return (
		<CoderAgentContext.Provider
			value={{
				enabled,
				open,
				toggle,
				close,
				messages,
				sendMessage,
				startNewChat,
				isThinking,
				chatId,
			}}
		>
			{children}
		</CoderAgentContext.Provider>
	);
};

export function useCoderAgentContext(): CoderAgentContextValue {
	const ctx = useContext(CoderAgentContext);
	if (!ctx) {
		throw new Error(
			"useCoderAgentContext must be used within a CoderAgentProvider",
		);
	}
	return ctx;
}
