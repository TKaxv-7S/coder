import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockBuiltinChatPersona,
	MockChatPersonas,
	MockDeploymentChatPersona,
	MockOrganizationChatPersona,
} from "#/testHelpers/chatAgents";
import { MockOrganization } from "#/testHelpers/entities";
import { ChatPersonasPageView } from "./ChatPersonasPageView";

const meta: Meta<typeof ChatPersonasPageView> = {
	title: "modules/chatAgents/ChatPersonasPageView",
	component: ChatPersonasPageView,
	args: {
		personas: MockChatPersonas,
		error: null,
		canEdit: true,
		isEntitled: true,
		onDelete: fn(),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/personas" },
			routing: [
				{ path: "/ai/settings/personas", useStoryElement: true },
				{ path: "/ai/settings/personas/create", useStoryElement: true },
				{ path: "/ai/settings/personas/:personaId", useStoryElement: true },
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof ChatPersonasPageView>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: /create persona/i }),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(MockBuiltinChatPersona.name),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(MockDeploymentChatPersona.name),
		).toBeInTheDocument();
		await expect(canvas.getByText("Builtin")).toBeInTheDocument();
		await expect(canvas.getByText("Deployment")).toBeInTheDocument();
		await expect(canvas.getByText("Organization")).toBeInTheDocument();
		// The builtin persona is read-only and gets no row actions;
		// the deployment persona (matching scope) does.
		await expect(
			canvas.queryByRole("button", {
				name: `Open menu for persona ${MockBuiltinChatPersona.name}`,
			}),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("button", {
				name: `Open menu for persona ${MockDeploymentChatPersona.name}`,
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
		// Deployment and builtin entries render read-only for reference;
		// only the org-scoped entry is editable.
		await expect(
			canvas.queryByRole("button", {
				name: `Open menu for persona ${MockDeploymentChatPersona.name}`,
			}),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("button", {
				name: `Open menu for persona ${MockOrganizationChatPersona.name}`,
			}),
		).toBeInTheDocument();
	},
};

export const Loading: Story = {
	args: {
		personas: undefined,
	},
};

export const Empty: Story = {
	args: {
		personas: [],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("No personas")).toBeInTheDocument();
	},
};

export const NotEntitled: Story = {
	args: {
		isEntitled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Chat personas")).toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: /create persona/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", {
				name: `Open menu for persona ${MockDeploymentChatPersona.name}`,
			}),
		).not.toBeInTheDocument();
	},
};

export const ReadOnlyForMembers: Story = {
	args: {
		canEdit: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("link", { name: /create persona/i }),
		).not.toBeInTheDocument();
	},
};
