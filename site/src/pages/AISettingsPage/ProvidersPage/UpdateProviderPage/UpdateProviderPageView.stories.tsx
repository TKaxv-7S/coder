import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type { AIProvider } from "#/api/typesGenerated";
import {
	MockAIProviderAnthropic,
	MockAIProviderBedrock,
	MockAIProviderCopilot,
	MockAIProviderOpenAI,
} from "#/testHelpers/entities";
import { withToaster } from "#/testHelpers/storybook";
import UpdateProviderPageView from "./UpdateProviderPageView";

const routingFor = (path: string) =>
	reactRouterParameters({
		location: { path },
		routing: [
			{ path: "/ai/settings/providers", useStoryElement: true },
			{ path: "/ai/settings/providers/:providerId", useStoryElement: true },
		],
	});

const seed = (provider: AIProvider) => ({
	queries: [{ key: ["ai", "providers", provider.name], data: provider }],
});

const meta: Meta<typeof UpdateProviderPageView> = {
	title: "pages/AISettingsPage/UpdateProviderPageView",
	component: UpdateProviderPageView,
	decorators: [withToaster],
};

export default meta;
type Story = StoryObj<typeof UpdateProviderPageView>;

export const OpenAI: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderOpenAI.name}`,
		),
		...seed(MockAIProviderOpenAI),
	},
};

export const Anthropic: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderAnthropic.name}`,
		),
		...seed(MockAIProviderAnthropic),
	},
};

export const Bedrock: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderBedrock.name}`,
		),
		...seed(MockAIProviderBedrock),
	},
};

// Regression coverage for AIGOV-482. A Bedrock provider migrated from
// chat_providers (migrations 000504/000505) arrives as `type=bedrock` with
// `settings=null` and, for env-var deployments, a blank base_url. The edit
// form must recognise it as Bedrock, fill the endpoint and model defaults,
// and stay submittable once the operator makes an edit.
const MockAIProviderBedrockMigrated: AIProvider = {
	...MockAIProviderBedrock,
	name: "agents-bedrock",
	display_name: "",
	base_url: "",
	api_keys: [],
	settings: null as unknown as AIProvider["settings"],
};

export const BedrockMigratedEmptySettings: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderBedrockMigrated.name}`,
		),
		...seed(MockAIProviderBedrockMigrated),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The Bedrock fields render (not the generic API-key layout) and the
		// endpoint/model defaults are pre-filled from the form defaults.
		const endpoint = await canvas.findByLabelText(/endpoint/i);
		expect(endpoint).toHaveValue(
			"https://bedrock-runtime.us-east-2.amazonaws.com",
		);
		expect(canvas.queryByLabelText(/api key/i)).not.toBeInTheDocument();
		const model = await canvas.findByLabelText(/^model\s*\*?$/i);
		expect(model).toHaveValue(
			"global.anthropic.claude-sonnet-4-5-20250929-v1:0",
		);

		// Making an edit enables the Update button: the form is valid despite
		// the migrated null settings and blank base_url.
		const displayName = canvas.getByLabelText(/display name/i);
		await userEvent.type(displayName, "Migrated Bedrock");
		const submitButton = canvas.getByRole("button", {
			name: /update provider/i,
		});
		await waitFor(() => expect(submitButton).toBeEnabled());
	},
};

// Copilot has no stored credential, so the edit form renders no API key
// field and keeps the immutable name disabled.
export const Copilot: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderCopilot.name}`,
		),
		...seed(MockAIProviderCopilot),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const name = await canvas.findByLabelText(/^name/i);
		expect(name).toBeDisabled();
		expect(canvas.queryByLabelText(/api key/i)).not.toBeInTheDocument();
	},
};

// No seeded query: the page renders the loader while useQuery fetches.
export const Loading: Story = {
	parameters: {
		reactRouter: routingFor("/ai/settings/providers/loading-provider"),
	},
};

export const DeleteDialogOpen: Story = {
	parameters: {
		reactRouter: routingFor(
			`/ai/settings/providers/${MockAIProviderOpenAI.name}`,
		),
		...seed(MockAIProviderOpenAI),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const deleteButton = await canvas.findByRole("button", {
			name: /^delete$/i,
		});
		await userEvent.click(deleteButton);
		// DeleteDialog renders via Radix portal, so search the document, not
		// just the story canvas.
		await expect(await screen.findByRole("dialog")).toBeInTheDocument();
		await expect(await screen.findByText(/irreversible/i)).toBeInTheDocument();
	},
};
