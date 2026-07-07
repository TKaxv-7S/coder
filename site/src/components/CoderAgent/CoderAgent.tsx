import { type FC, useEffect, useState } from "react";
import { CoderAgentButton } from "./CoderAgentButton";
import { CoderAgentPanel } from "./CoderAgentPanel";
import { useCoderAgentContext } from "./CoderAgentProvider";

export const CoderAgent: FC = () => {
	const {
		enabled,
		open,
		toggle,
		close,
		messages,
		sendMessage,
		startNewChat,
		isThinking,
	} = useCoderAgentContext();

	// Track how many messages the user has seen so the unread
	// indicator only pulses for genuinely new messages.
	const [seenCount, setSeenCount] = useState(0);

	useEffect(() => {
		if (open) {
			setSeenCount(messages.length);
		}
	}, [open, messages.length]);

	if (!enabled) {
		return null;
	}

	return (
		<>
			<CoderAgentPanel
				open={open}
				onClose={close}
				onNewChat={startNewChat}
				messages={messages}
				onSendMessage={sendMessage}
				isThinking={isThinking}
			/>
			<CoderAgentButton
				open={open}
				onToggle={toggle}
				isThinking={isThinking}
				hasUnread={messages.length > seenCount && !open}
			/>
		</>
	);
};
