import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	MockBuiltinChatPersona,
	MockDeploymentChatPersona,
} from "#/testHelpers/chatAgents";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import { ChatPersonaForm } from "./ChatPersonaForm";

const meta: Meta<typeof ChatPersonaForm> = {
	title: "modules/chatAgents/ChatPersonaForm",
	component: ChatPersonaForm,
	args: {
		modelConfigs: [MockChatModelConfig],
		isSaving: false,
		error: null,
		onSubmit: fn(),
		onCancel: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatPersonaForm>;

export const Create: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Slug")).toBeEnabled();
		await expect(
			canvas.getByRole("button", { name: /create persona/i }),
		).toBeInTheDocument();
	},
};

export const CreateValidation: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /create persona/i }),
		);
		await expect(canvas.getByText("Name is required.")).toBeVisible();
		await expect(canvas.getByText("Slug is required.")).toBeVisible();
		await expect(canvas.getByText("System prompt is required.")).toBeVisible();
		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

export const SlugFormatValidation: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText("Slug"), "Not A Slug");
		await userEvent.click(
			canvas.getByRole("button", { name: /create persona/i }),
		);
		await expect(
			canvas.getByText("Slug must be lowercase letters, numbers, and hyphens."),
		).toBeVisible();
	},
};

export const SubmitCreate: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText("Name"), "Docs Writer");
		await userEvent.type(canvas.getByLabelText("Slug"), "docs-writer");
		await userEvent.type(
			canvas.getByLabelText("System prompt"),
			"You write documentation.",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: /create persona/i }),
		);
		await expect(args.onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({
				name: "Docs Writer",
				slug: "docs-writer",
				system_prompt: "You write documentation.",
				enabled: true,
			}),
		);
	},
};

export const Edit: Story = {
	args: {
		editingPersona: MockDeploymentChatPersona,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The slug is immutable on edit.
		await expect(canvas.getByLabelText("Slug")).toBeDisabled();
		await expect(canvas.getByLabelText("Name")).toHaveValue(
			MockDeploymentChatPersona.name,
		);
		await expect(
			canvas.getByRole("button", { name: /save persona/i }),
		).toBeInTheDocument();
	},
};

export const ReadOnlyBuiltin: Story = {
	args: {
		editingPersona: MockBuiltinChatPersona,
		readOnly: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Name")).toBeDisabled();
		await expect(canvas.getByLabelText("System prompt")).toBeDisabled();
		await expect(
			canvas.queryByRole("button", { name: /save persona/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("button", { name: "Back" }),
		).toBeInTheDocument();
	},
};
