import { type FC, useEffect, useState } from "react";
import {
	selectOrderedMessageIDs,
	useChatSelector,
} from "#/pages/AgentsPage/components/ChatConversation/chatStore";
import { CoderAgentButton } from "./CoderAgentButton";
import { CoderAgentPanel } from "./CoderAgentPanel";
import { useCoderAgentContext } from "./CoderAgentProvider";

export const CoderAgent: FC = () => {
	const {
		enabled,
		open,
		toggle,
		close,
		chatId,
		store,
		persistedError,
		sendMessage,
		startNewChat,
		isThinking,
	} = useCoderAgentContext();

	const orderedMessageIDs = useChatSelector(store, selectOrderedMessageIDs);
	const messageCount = orderedMessageIDs.length;

	// Track how many messages the user has seen so the unread
	// indicator only pulses for genuinely new messages.
	const [seenCount, setSeenCount] = useState(0);

	useEffect(() => {
		if (open) {
			setSeenCount(messageCount);
		}
	}, [open, messageCount]);

	if (!enabled) {
		return null;
	}

	return (
		<>
			<CoderAgentPanel
				open={open}
				onClose={close}
				onNewChat={startNewChat}
				onSendMessage={sendMessage}
				isThinking={isThinking}
				chatId={chatId}
				store={store}
				persistedError={persistedError}
			/>
			<CoderAgentButton
				open={open}
				onToggle={toggle}
				isThinking={isThinking}
				hasUnread={messageCount > seenCount && !open}
			/>
		</>
	);
};
