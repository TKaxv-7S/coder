import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, within } from "storybook/test";
import {
	MockBuiltinAssistantChatAgent,
	MockBuiltinChatAgent,
	MockChatAgents,
	MockDeploymentChatAgent,
} from "#/testHelpers/chatAgents";
import { AgentSelector } from "./AgentSelector";

const meta: Meta<typeof AgentSelector> = {
	title: "pages/AgentsPage/ChatElements/AgentSelector",
	component: AgentSelector,
	args: {
		options: MockChatAgents.filter((agent) => agent.enabled),
		value: MockBuiltinChatAgent.id,
		onValueChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AgentSelector>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: MockBuiltinChatAgent.name }),
		).toBeInTheDocument();
	},
};

export const OpenList: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: MockBuiltinChatAgent.name }),
		);
		const listbox = await screen.findByRole("listbox");
		const list = within(listbox);
		await expect(
			list.getByRole("option", {
				name: (name) => name.includes(MockBuiltinChatAgent.name),
			}),
		).toBeInTheDocument();
		await expect(
			list.getByRole("option", {
				name: (name) => name.includes(MockBuiltinAssistantChatAgent.name),
			}),
		).toBeInTheDocument();
		await expect(
			list.getByRole("option", {
				name: (name) => name.includes(MockDeploymentChatAgent.name),
			}),
		).toBeInTheDocument();
	},
};

export const SelectAgent: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: MockBuiltinChatAgent.name }),
		);
		const listbox = await screen.findByRole("listbox");
		await userEvent.click(
			within(listbox).getByText(MockDeploymentChatAgent.name),
		);
		await expect(args.onValueChange).toHaveBeenCalledWith(
			MockDeploymentChatAgent.id,
		);
	},
};

export const SearchFiltersAgents: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: MockBuiltinChatAgent.name }),
		);
		const body = within(document.body);
		const search = body.getByPlaceholderText("Search...");
		await userEvent.type(search, "support");
		const listbox = await screen.findByRole("listbox");
		const list = within(listbox);
		await expect(list.getByText(MockDeploymentChatAgent.name)).toBeVisible();
		await expect(
			list.queryByText(MockBuiltinAssistantChatAgent.name),
		).not.toBeInTheDocument();
	},
};

export const Empty: Story = {
	args: {
		options: [],
		value: "",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: "Select agent" }),
		).toBeDisabled();
	},
};

export const Disabled: Story = {
	args: {
		disabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: MockBuiltinChatAgent.name }),
		).toBeDisabled();
	},
};
