import type { Meta, StoryObj } from "@storybook/react-vite";
import { Truncate } from "./Truncate";

const meta: Meta<typeof Truncate> = {
	title: "components/Truncate",
	component: Truncate,
	args: {
		children:
			"bedrock-runtime.us-east-2.amazonaws.com/model/anthropic.claude-3-5-sonnet-20241022-v2:0/invoke",
	},
	decorators: [
		(Story) => (
			<div className="max-w-96 rounded border border-solid border-border-default bg-surface-primary p-3 text-xs text-content-primary">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof Truncate>;

export const End: Story = {
	args: { position: "end" },
};

export const Middle: Story = {
	args: { position: "middle" },
};

export const Start: Story = {
	args: { position: "start" },
};

export const Short: Story = {
	args: { position: "middle", children: "short" },
};

export const Empty: Story = {
	args: { position: "middle", children: "" },
};

export const CustomEllipsis: Story = {
	args: { position: "middle", ellipsis: " ... " },
};

// Multi-code-unit characters (emoji, combined marks) must never be cut in
// half. The binary search operates on grapheme clusters, so the ellipsis
// only ever lands between clusters.
export const Emoji: Story = {
	args: {
		position: "middle",
		children:
			"👨‍👩‍👧‍👦 family emoji is one grapheme, followed by more text that will need to be truncated 👨‍👩‍👧‍👦",
	},
};

export const Narrow: Story = {
	decorators: [
		(Story) => (
			<div className="max-w-24 rounded border border-solid border-border-default bg-surface-primary p-3 text-xs text-content-primary">
				<Story />
			</div>
		),
	],
	args: { position: "middle" },
};

// When even the ellipsis will not fit, Truncate renders the empty string
// rather than overflowing the container.
export const TooNarrowForEllipsis: Story = {
	decorators: [
		(Story) => (
			<div
				className="rounded border border-solid border-border-default bg-surface-primary p-3 text-xs text-content-primary"
				style={{ width: "8px" }}
			>
				<Story />
			</div>
		),
	],
	args: { position: "middle" },
};

// Common shape: an icon plus a filepath/URL inside a fixed-width container.
// The Truncate consumes the flex slack so the icon stays visible.
export const InFlexRow: Story = {
	render: (args) => (
		<div className="flex items-center gap-2 min-w-0">
			<span
				aria-hidden
				className="inline-block h-3 w-3 shrink-0 rounded-sm bg-content-link"
			/>
			<Truncate {...args} />
		</div>
	),
	args: {
		position: "middle",
		children:
			"/very/long/monorepo/path/site/src/pages/AgentsPage/components/ChatElements/tools/DiffFileHeader.tsx",
	},
};
