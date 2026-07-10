import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockBuiltinChatAgent,
	MockBuiltinChatPersona,
	MockChatAgents,
	MockChatPersonas,
	MockDeploymentChatAgent,
	MockOrganizationChatAgent,
} from "#/testHelpers/chatAgents";
import { MockOrganization } from "#/testHelpers/entities";
import { ChatAgentsPageView } from "./ChatAgentsPageView";

const meta: Meta<typeof ChatAgentsPageView> = {
	title: "modules/chatAgents/ChatAgentsPageView",
	component: ChatAgentsPageView,
	args: {
		agents: MockChatAgents,
		personas: MockChatPersonas,
		error: null,
		canEdit: true,
		isEntitled: true,
		onDelete: fn(),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/agents" },
			routing: [
				{ path: "/ai/settings/agents", useStoryElement: true },
				{ path: "/ai/settings/agents/create", useStoryElement: true },
				{ path: "/ai/settings/agents/:agentId", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ChatAgentsPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: /create agent/i }),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(MockBuiltinChatAgent.name),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(MockDeploymentChatAgent.name),
		).toBeInTheDocument();
		// The agent's persona name resolves through the personas list.
		await expect(
			canvas.getAllByText(MockBuiltinChatPersona.name).length,
		).toBeGreaterThan(0);
		// Builtin rows are read-only, deployment rows editable.
		await expect(
			canvas.queryByRole("button", {
				name: `Open menu for agent ${MockBuiltinChatAgent.name}`,
			}),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("button", {
				name: `Open menu for agent ${MockDeploymentChatAgent.name}`,
			}),
		).toBeInTheDocument();
	},
};

export const OrganizationScope: Story = {
	args: {
		organizationId: MockOrganization.id,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", {
				name: `Open menu for agent ${MockDeploymentChatAgent.name}`,
			}),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("button", {
				name: `Open menu for agent ${MockOrganizationChatAgent.name}`,
			}),
		).toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: {
		agents: undefined,
	},
};

export const Empty: Story = {
	args: {
		agents: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("No agents")).toBeInTheDocument();
	},
};

export const NotEntitled: Story = {
	args: {
		isEntitled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Chat agents")).toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: /create agent/i }),
		).not.toBeInTheDocument();
	},
};
