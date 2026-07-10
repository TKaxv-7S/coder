import { useFormik } from "formik";
import type { FC, ReactNode } from "react";
import * as Yup from "yup";
import type { ChatModelConfig } from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "#/components/Select/Select";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import { Textarea } from "#/components/Textarea/Textarea";

export const slugValidationSchema = Yup.string()
	.trim()
	.required("Slug is required.")
	.matches(
		/^[a-z0-9]+(-[a-z0-9]+)*$/,
		"Slug must be lowercase letters, numbers, and hyphens.",
	);

// Sentinel for the "no model" select option. Radix Select items cannot
// have an empty string value.
const NO_MODEL_VALUE = "none";

interface FieldProps {
	label: string;
	htmlFor: string;
	error?: string;
	children: ReactNode;
}

export const FormField: FC<FieldProps> = ({
	label,
	htmlFor,
	error,
	children,
}) => (
	<div className="flex flex-col gap-1.5">
		<Label htmlFor={htmlFor} className="text-sm text-content-primary">
			{label}
		</Label>
		{children}
		{error && <p className="m-0 text-xs text-content-destructive">{error}</p>}
	</div>
);

interface ModelSelectFieldProps {
	id: string;
	label: string;
	value: string;
	modelConfigs: readonly ChatModelConfig[];
	disabled?: boolean;
	noneLabel: string;
	onChange: (value: string) => void;
}

export const ModelSelectField: FC<ModelSelectFieldProps> = ({
	id,
	label,
	value,
	modelConfigs,
	disabled,
	noneLabel,
	onChange,
}) => (
	<FormField label={label} htmlFor={id}>
		<Select
			value={value || NO_MODEL_VALUE}
			onValueChange={(next) => onChange(next === NO_MODEL_VALUE ? "" : next)}
			disabled={disabled}
		>
			<SelectTrigger id={id} aria-label={label}>
				<SelectValue placeholder={noneLabel} />
			</SelectTrigger>
			<SelectContent>
				<SelectItem value={NO_MODEL_VALUE}>{noneLabel}</SelectItem>
				{modelConfigs.map((config) => (
					<SelectItem key={config.id} value={config.id}>
						{config.display_name || config.model}
					</SelectItem>
				))}
			</SelectContent>
		</Select>
	</FormField>
);

interface EnabledFieldProps {
	checked: boolean;
	disabled?: boolean;
	onCheckedChange: (checked: boolean) => void;
}

export const EnabledField: FC<EnabledFieldProps> = ({
	checked,
	disabled,
	onCheckedChange,
}) => (
	<div className="flex items-center gap-2">
		<Switch
			id="enabled"
			checked={checked}
			onCheckedChange={onCheckedChange}
			disabled={disabled}
			aria-label="Enabled"
		/>
		<Label htmlFor="enabled" className="text-sm text-content-primary">
			Enabled
		</Label>
	</div>
);

export interface ChatPersonaFormValues {
	name: string;
	slug: string;
	description: string;
	icon: string;
	system_prompt: string;
	model_config_id: string;
	enabled: boolean;
}

interface ChatPersonaFormValuesSource {
	readonly name: string;
	readonly slug: string;
	readonly description: string;
	readonly icon: string;
	readonly system_prompt: string;
	readonly model_config_id?: string;
	readonly enabled: boolean;
}

const validationSchema = Yup.object({
	name: Yup.string().trim().required("Name is required."),
	slug: slugValidationSchema,
	system_prompt: Yup.string().trim().required("System prompt is required."),
});

interface ChatPersonaFormProps {
	editingPersona?: ChatPersonaFormValuesSource;
	modelConfigs: readonly ChatModelConfig[];
	isSaving: boolean;
	// Builtin entries render the form disabled with no save action.
	readOnly?: boolean;
	error: unknown;
	onSubmit: (values: ChatPersonaFormValues) => Promise<void>;
	onCancel: () => void;
}

export const ChatPersonaForm: FC<ChatPersonaFormProps> = ({
	editingPersona,
	modelConfigs,
	isSaving,
	readOnly = false,
	error,
	onSubmit,
	onCancel,
}) => {
	const isEditing = Boolean(editingPersona);
	const form = useFormik<ChatPersonaFormValues>({
		initialValues: {
			name: editingPersona?.name ?? "",
			slug: editingPersona?.slug ?? "",
			description: editingPersona?.description ?? "",
			icon: editingPersona?.icon ?? "",
			system_prompt: editingPersona?.system_prompt ?? "",
			model_config_id: editingPersona?.model_config_id ?? "",
			enabled: editingPersona?.enabled ?? true,
		},
		validationSchema,
		onSubmit: async (values) => {
			await onSubmit(values);
		},
	});

	const fieldError = (field: keyof ChatPersonaFormValues) =>
		form.touched[field] && form.errors[field]
			? String(form.errors[field])
			: undefined;

	return (
		<form
			onSubmit={form.handleSubmit}
			aria-label="Persona form"
			className="flex max-w-2xl flex-col gap-5"
		>
			{Boolean(error) && <ErrorAlert error={error} />}
			<FormField label="Name" htmlFor="name" error={fieldError("name")}>
				<Input
					id="name"
					{...form.getFieldProps("name")}
					disabled={readOnly}
					placeholder="Code Reviewer"
				/>
			</FormField>
			<FormField label="Slug" htmlFor="slug" error={fieldError("slug")}>
				<Input
					id="slug"
					{...form.getFieldProps("slug")}
					disabled={readOnly || isEditing}
					placeholder="code-reviewer"
				/>
			</FormField>
			<FormField label="Description" htmlFor="description">
				<Input
					id="description"
					{...form.getFieldProps("description")}
					disabled={readOnly}
					placeholder="What this persona is for"
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
				label="System prompt"
				htmlFor="system_prompt"
				error={fieldError("system_prompt")}
			>
				<Textarea
					id="system_prompt"
					{...form.getFieldProps("system_prompt")}
					disabled={readOnly}
					rows={10}
					placeholder="You are a helpful assistant..."
				/>
			</FormField>
			<ModelSelectField
				id="model_config_id"
				label="Model preference"
				value={form.values.model_config_id}
				modelConfigs={modelConfigs}
				disabled={readOnly}
				noneLabel="Chat default"
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
						{isEditing ? "Save persona" : "Create persona"}
					</Button>
				)}
			</div>
		</form>
	);
};
