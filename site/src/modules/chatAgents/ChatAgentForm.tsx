import { useFormik } from "formik";
import type { FC } from "react";
import * as Yup from "yup";
import type { ChatModelConfig, ChatPersona } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Textarea } from "#/components/Textarea/Textarea";
import {
	EnabledField,
	FormField,
	ModelSelectField,
	slugValidationSchema,
} from "./ChatPersonaForm";

export interface ChatAgentFormValues {
	name: string;
	slug: string;
	description: string;
	icon: string;
	persona_id: string;
	prompt_append: string;
	model_config_id: string;
	enabled: boolean;
}

interface ChatAgentFormValuesSource {
	readonly name: string;
	readonly slug: string;
	readonly description: string;
	readonly icon: string;
	readonly persona_id: string;
	readonly prompt_append: string;
	readonly model_config_id?: string;
	readonly enabled: boolean;
}

const validationSchema = Yup.object({
	name: Yup.string().trim().required("Name is required."),
	slug: slugValidationSchema,
	persona_id: Yup.string().required("Persona is required."),
});

interface ChatAgentFormProps {
	editingAgent?: ChatAgentFormValuesSource;
	personas: readonly ChatPersona[];
	modelConfigs: readonly ChatModelConfig[];
	isSaving: boolean;
	// Builtin entries render the form disabled with no save action.
	readOnly?: boolean;
	error: unknown;
	onSubmit: (values: ChatAgentFormValues) => Promise<void>;
	onCancel: () => void;
}

export const ChatAgentForm: FC<ChatAgentFormProps> = ({
	editingAgent,
	personas,
	modelConfigs,
	isSaving,
	readOnly = false,
	error,
	onSubmit,
	onCancel,
}) => {
	const isEditing = Boolean(editingAgent);
	const form = useFormik<ChatAgentFormValues>({
		initialValues: {
			name: editingAgent?.name ?? "",
			slug: editingAgent?.slug ?? "",
			description: editingAgent?.description ?? "",
			icon: editingAgent?.icon ?? "",
			persona_id: editingAgent?.persona_id ?? "",
			prompt_append: editingAgent?.prompt_append ?? "",
			model_config_id: editingAgent?.model_config_id ?? "",
			enabled: editingAgent?.enabled ?? true,
		},
		validationSchema,
		onSubmit: async (values) => {
			await onSubmit(values);
		},
	});

	const fieldError = (field: keyof ChatAgentFormValues) =>
		form.touched[field] && form.errors[field]
			? String(form.errors[field])
			: undefined;

	return (
		<form
			onSubmit={form.handleSubmit}
			aria-label="Agent form"
			className="flex max-w-2xl flex-col gap-5"
		>
			{Boolean(error) && <ErrorAlert error={error} />}
			<FormField label="Name" htmlFor="name" error={fieldError("name")}>
				<Input
					id="name"
					{...form.getFieldProps("name")}
					disabled={readOnly}
					placeholder="Reviewer"
				/>
			</FormField>
			<FormField label="Slug" htmlFor="slug" error={fieldError("slug")}>
				<Input
					id="slug"
					{...form.getFieldProps("slug")}
					disabled={readOnly || isEditing}
					placeholder="reviewer"
				/>
			</FormField>
			<FormField label="Description" htmlFor="description">
				<Input
					id="description"
					{...form.getFieldProps("description")}
					disabled={readOnly}
					placeholder="What this agent is for"
				/>
			</FormField>
			<FormField label="Icon" htmlFor="icon">
				<Input
					id="icon"
					{...form.getFieldProps("icon")}
					disabled={readOnly}
					placeholder="/emojis/1f916.png"
				/>
			</FormField>
			<FormField
				label="Persona"
				htmlFor="persona_id"
				error={fieldError("persona_id")}
			>
				<Select
					value={form.values.persona_id || undefined}
					onValueChange={(value) => form.setFieldValue("persona_id", value)}
					disabled={readOnly}
				>
					<SelectTrigger id="persona_id" aria-label="Persona">
						<SelectValue placeholder="Select a persona" />
					</SelectTrigger>
					<SelectContent>
						{personas.map((persona) => (
							<SelectItem key={persona.id} value={persona.id}>
								{persona.name || persona.slug}
							</SelectItem>
						))}
					</SelectContent>
				</Select>
			</FormField>
			<FormField label="Prompt append" htmlFor="prompt_append">
				<Textarea
					id="prompt_append"
					{...form.getFieldProps("prompt_append")}
					disabled={readOnly}
					rows={6}
					placeholder="Additional instructions appended after the persona prompt (optional)"
				/>
			</FormField>
			<ModelSelectField
				id="model_config_id"
				label="Model override"
				value={form.values.model_config_id}
				modelConfigs={modelConfigs}
				disabled={readOnly}
				noneLabel="Persona default"
				onChange={(value) => form.setFieldValue("model_config_id", value)}
			/>
			<EnabledField
				checked={form.values.enabled}
				disabled={readOnly}
				onCheckedChange={(checked) => form.setFieldValue("enabled", checked)}
			/>
			<div className="flex items-center gap-2">
				<Button type="button" variant="outline" onClick={onCancel}>
					{readOnly ? "Back" : "Cancel"}
				</Button>
				{!readOnly && (
					<Button type="submit" disabled={isSaving}>
						{isSaving && <Spinner loading />}
						{isEditing ? "Save agent" : "Create agent"}
					</Button>
				)}
			</div>
		</form>
	);
};
