import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	MockBuiltinChatAgent,
	MockChatPersonas,
	MockDeploymentChatAgent,
	MockDeploymentChatPersona,
} from "#/testHelpers/chatAgents";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import { ChatAgentForm } from "./ChatAgentForm";

const meta: Meta<typeof ChatAgentForm> = {
	title: "modules/chatAgents/ChatAgentForm",
	component: ChatAgentForm,
	args: {
		personas: MockChatPersonas,
		modelConfigs: [MockChatModelConfig],
		isSaving: false,
		error: null,
		onSubmit: fn(),
		onCancel: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ChatAgentForm>;

export const Create: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Slug")).toBeEnabled();
		await expect(
			canvas.getByRole("combobox", { name: "Persona" }),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("button", { name: /create agent/i }),
		).toBeInTheDocument();
	},
};

export const CreateValidation: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /create agent/i }),
		);
		await expect(canvas.getByText("Name is required.")).toBeVisible();
		await expect(canvas.getByText("Slug is required.")).toBeVisible();
		await expect(canvas.getByText("Persona is required.")).toBeVisible();
		await expect(args.onSubmit).not.toHaveBeenCalled();
	},
};

export const SubmitCreate: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText("Name"), "Docs Agent");
		await userEvent.type(canvas.getByLabelText("Slug"), "docs-agent");
		await userEvent.click(canvas.getByRole("combobox", { name: "Persona" }));
		const option = await within(document.body).findByRole("option", {
			name: MockDeploymentChatPersona.name,
		});
		await userEvent.click(option);
		await userEvent.click(
			canvas.getByRole("button", { name: /create agent/i }),
		);
		await expect(args.onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({
				name: "Docs Agent",
				slug: "docs-agent",
				persona_id: MockDeploymentChatPersona.id,
				enabled: true,
			}),
		);
	},
};

export const Edit: Story = {
	args: {
		editingAgent: MockDeploymentChatAgent,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The slug is immutable on edit.
		await expect(canvas.getByLabelText("Slug")).toBeDisabled();
		await expect(canvas.getByLabelText("Name")).toHaveValue(
			MockDeploymentChatAgent.name,
		);
		await expect(canvas.getByLabelText("Prompt append")).toHaveValue(
			MockDeploymentChatAgent.prompt_append,
		);
		await expect(
			canvas.getByRole("button", { name: /save agent/i }),
		).toBeInTheDocument();
	},
};

export const ReadOnlyBuiltin: Story = {
	args: {
		editingAgent: MockBuiltinChatAgent,
		readOnly: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Name")).toBeDisabled();
		await expect(canvas.getByLabelText("Prompt append")).toBeDisabled();
		await expect(
			canvas.queryByRole("button", { name: /save agent/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("button", { name: "Back" }),
		).toBeInTheDocument();
	},
};
