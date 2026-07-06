import type { FC } from "react";
import { BlinkButton } from "./BlinkButton";
import { BlinkPanel } from "./BlinkPanel";
import { useBlinkContext } from "./BlinkProvider";

export const Blink: FC = () => {
	const {
		enabled,
		open,
		toggle,
		close,
		messages,
		sendMessage,
		startNewChat,
		isThinking,
	} = useBlinkContext();

	if (!enabled) {
		return null;
	}

	return (
		<>
			<BlinkPanel
				open={open}
				onClose={close}
				onNewChat={startNewChat}
				messages={messages}
				onSendMessage={sendMessage}
				isThinking={isThinking}
			/>
			<BlinkButton
				open={open}
				onToggle={toggle}
				isThinking={isThinking}
				hasUnread={messages.length > 0 && !open}
			/>
		</>
	);
};
