import type * as TypesGen from "#/api/typesGenerated";
import {
	filterByNameAndDescription,
	personalSkillTriggerText,
} from "./personalSkills";

/**
 * A built-in chat command offered by the "/" trigger menu. Unlike
 * personal skills, commands are fixed client-side actions: the
 * composer intercepts them at submit time instead of sending the
 * text as a message.
 */
export type ChatSlashCommand = {
	name: string;
	description: string;
};

export const COMPACT_SLASH_COMMAND: ChatSlashCommand = {
	name: "compact",
	description:
		"Summarize the conversation so far to free up context window space",
};

/**
 * Commands available in the main chat composer. Editing an existing
 * message and the new-agent form do not offer commands.
 */
export const CHAT_SLASH_COMMANDS: readonly ChatSlashCommand[] = [
	COMPACT_SLASH_COMMAND,
];

export const chatSlashCommandTriggerText = (
	command: ChatSlashCommand,
): string => `/${command.name}`;

/**
 * One entry in the "/" trigger menu: either a built-in command or a
 * personal skill. Keyboard selection indexes into the combined list,
 * commands first.
 */
export type SlashMenuItem =
	| { kind: "command"; command: ChatSlashCommand }
	| { kind: "skill"; skill: TypesGen.UserSkillMetadata };

/** Unique cmdk value for an item; command and skill names may collide. */
export const slashMenuItemValue = (item: SlashMenuItem): string =>
	item.kind === "command"
		? `command:${item.command.name}`
		: `skill:${item.skill.name}`;

/** The "/name" text inserted into the composer when an item is chosen. */
export const slashMenuItemTriggerText = (item: SlashMenuItem): string =>
	item.kind === "command"
		? chatSlashCommandTriggerText(item.command)
		: personalSkillTriggerText(item.skill);

/**
 * Filters commands for the current trigger query with the same
 * ranking as personal skills.
 */
export const filterChatSlashCommands = (
	commands: readonly ChatSlashCommand[],
	query: string,
): ChatSlashCommand[] => filterByNameAndDescription(commands, query);
