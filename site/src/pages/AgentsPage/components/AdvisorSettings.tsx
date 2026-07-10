import { useFormik } from "formik";
import { type FC, useId } from "react";
import { getErrorMessage } from "#/api/errors";
import type {
	AdvisorConfig,
	ChatModelConfig,
	UpdateAdvisorConfigRequest,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { useTemporarySavedState } from "#/components/TemporarySavedState/TemporarySavedState";
import { AgentSettingLayout } from "#/pages/AISettingsPage/CoderAgentsPage/components/AgentSettingLayout";
import { cn } from "#/utils/cn";
import type { ProviderInfo } from "../utils/modelOptions";
import { pickReasoningEffort } from "../utils/reasoningEffort";
import {
	ModelSelector,
	type ModelSelectorOption,
	type ModelSelectorSpecialOption,
} from "./ChatElements";

const nilUUID = "00000000-0000-0000-0000-000000000000";
const chatModelFallbackValue = "__use-chat-model__";

interface MutationCallbacks {
	onSuccess?: () => void;
	onError?: () => void;
}

interface AdvisorSettingsProps {
	advisorConfigData: AdvisorConfig | undefined;
	isAdvisorConfigLoading: boolean;
	isAdvisorConfigFetching: boolean;
	isAdvisorConfigLoadError: boolean;
	modelConfigs: readonly ChatModelConfig[];
	providerInfoByID: ReadonlyMap<string, ProviderInfo>;
	modelConfigsError: unknown;
	isLoadingModelConfigs: boolean;
	isFetchingModelConfigs: boolean;
	onSaveAdvisorConfig: (
		req: UpdateAdvisorConfigRequest,
		options?: MutationCallbacks,
	) => void;
	isSavingAdvisorConfig: boolean;
	isSaveAdvisorConfigError: boolean;
	saveAdvisorConfigError: unknown;
}

type AdvisorSettingsFormValues = {
	max_uses_per_run: string;
	max_output_tokens: string;
	model_config_id: string;
	reasoning_effort: string;
};

const isUnsetModelConfigId = (id: string): boolean =>
	id === "" || id === nilUUID;

const normalizeNonNegativeInteger = (
	value: number | string | undefined,
): number => {
	const parsed = typeof value === "number" ? value : Number(value);
	if (!Number.isFinite(parsed) || parsed < 0) {
		return 0;
	}
	return Math.trunc(parsed);
};

const normalizeAdvisorConfig = (
	config: AdvisorConfig | undefined,
): AdvisorSettingsFormValues => ({
	max_uses_per_run: String(
		normalizeNonNegativeInteger(config?.max_uses_per_run),
	),
	max_output_tokens: String(
		normalizeNonNegativeInteger(config?.max_output_tokens),
	),
	model_config_id:
		typeof config?.model_config_id === "string" &&
		!isUnsetModelConfigId(config.model_config_id)
			? config.model_config_id
			: "",
	reasoning_effort:
		typeof config?.reasoning_effort === "string" ? config.reasoning_effort : "",
});

const toAdvisorConfigRequest = (
	values: AdvisorSettingsFormValues,
): UpdateAdvisorConfigRequest => ({
	enabled: true,
	max_uses_per_run: normalizeNonNegativeInteger(values.max_uses_per_run),
	max_output_tokens: normalizeNonNegativeInteger(values.max_output_tokens),
	model_config_id: isUnsetModelConfigId(values.model_config_id)
		? nilUUID
		: values.model_config_id,
	...(!isUnsetModelConfigId(values.model_config_id) && values.reasoning_effort
		? { reasoning_effort: values.reasoning_effort }
		: {}),
});

const isNonNegativeIntegerString = (value: string): boolean => {
	if (value.trim() === "") {
		return false;
	}
	const parsed = Number(value);
	return Number.isFinite(parsed) && parsed >= 0 && Number.isInteger(parsed);
};

const validateAdvisorConfig = (values: AdvisorSettingsFormValues) => {
	const errors: Partial<Record<keyof AdvisorSettingsFormValues, string>> = {};

	if (!isNonNegativeIntegerString(values.max_uses_per_run)) {
		errors.max_uses_per_run =
			"Max uses per turn must be a non-negative integer.";
	}

	if (!isNonNegativeIntegerString(values.max_output_tokens)) {
		errors.max_output_tokens =
			"Max output tokens must be a non-negative integer.";
	}

	return errors;
};

const getModelDisplayName = (config: ChatModelConfig): string =>
	config.display_name.trim() || config.model;

export const AdvisorSettings: FC<AdvisorSettingsProps> = ({
	advisorConfigData,
	isAdvisorConfigLoading,
	isAdvisorConfigFetching,
	isAdvisorConfigLoadError,
	modelConfigs,
	providerInfoByID,
	modelConfigsError,
	isLoadingModelConfigs,
	isFetchingModelConfigs,
	onSaveAdvisorConfig,
	isSavingAdvisorConfig,
	isSaveAdvisorConfigError,
	saveAdvisorConfigError,
}) => {
	const maxUsesId = useId();
	const maxOutputTokensId = useId();
	const { isSavedVisible, showSavedState } = useTemporarySavedState();
	const hasLoadedAdvisorConfig = advisorConfigData !== undefined;
	const enabledModelConfigs = modelConfigs.filter((config) => config.enabled);
	const modelOptions: ModelSelectorOption[] = enabledModelConfigs.map(
		(modelConfig) => {
			const providerInfo = providerInfoByID.get(modelConfig.ai_provider_id);
			const reasoningEffort = modelConfig.model_config?.reasoning_effort;
			const reasoningEfforts = modelConfig.reasoning_efforts ?? [];
			return {
				id: modelConfig.id,
				provider: providerInfo?.provider ?? "",
				providerId: modelConfig.ai_provider_id,
				providerLabel: providerInfo?.displayName,
				providerIcon: providerInfo?.icon,
				model: modelConfig.model,
				displayName: getModelDisplayName(modelConfig),
				contextLimit: modelConfig.context_limit,
				...(reasoningEffort?.default
					? { reasoningEffortDefault: reasoningEffort.default }
					: {}),
				...(reasoningEfforts.length > 0 ? { reasoningEfforts } : {}),
			};
		},
	);

	const form = useFormik<AdvisorSettingsFormValues>({
		enableReinitialize: true,
		validateOnMount: true,
		initialValues: normalizeAdvisorConfig(advisorConfigData),
		validate: validateAdvisorConfig,
		onSubmit: (values, { resetForm }) => {
			// If the last committed model override references a model config
			// that no longer exists, the backend rejects the stale ID with a
			// 400. Clear the override so a save stays reliable in that edge
			// case. Only scrub when model configs have loaded successfully and
			// no refetch is in flight.
			let source = values;
			if (
				!isUnsetModelConfigId(source.model_config_id) &&
				!isLoadingModelConfigs &&
				!isFetchingModelConfigs &&
				!modelConfigsError &&
				!modelConfigs.some((config) => config.id === source.model_config_id)
			) {
				source = { ...source, model_config_id: "", reasoning_effort: "" };
			}
			const selectedOption = modelOptions.find(
				(option) => option.id === source.model_config_id,
			);
			if (
				source.reasoning_effort &&
				selectedOption &&
				!selectedOption.reasoningEfforts?.includes(source.reasoning_effort)
			) {
				source = { ...source, reasoning_effort: "" };
			}
			const request = toAdvisorConfigRequest(source);
			onSaveAdvisorConfig(request, {
				onSuccess: () => {
					const nextValues = normalizeAdvisorConfig(request);
					showSavedState();
					resetForm({ values: nextValues });
				},
			});
		},
	});

	const isFormDisabled =
		isSavingAdvisorConfig ||
		isAdvisorConfigLoading ||
		isAdvisorConfigFetching ||
		!hasLoadedAdvisorConfig;
	const isModelSelectDisabled =
		isFormDisabled || isLoadingModelConfigs || Boolean(modelConfigsError);
	const hasUnavailableSelectedModel =
		!isLoadingModelConfigs &&
		!isUnsetModelConfigId(form.values.model_config_id) &&
		!enabledModelConfigs.some(
			(config) => config.id === form.values.model_config_id,
		);
	const selectedModelConfig = modelConfigs.find(
		(config) => config.id === form.values.model_config_id,
	);
	const selectedModelOption = modelOptions.find(
		(option) => option.id === form.values.model_config_id,
	);
	const selectedReasoningEffort = selectedModelOption
		? pickReasoningEffort(
				form.values.reasoning_effort,
				selectedModelOption.reasoningEfforts ?? [],
				selectedModelOption.reasoningEffortDefault,
			)
		: undefined;
	const selectedModelValue = isUnsetModelConfigId(form.values.model_config_id)
		? chatModelFallbackValue
		: form.values.model_config_id;
	const specialOptions: ModelSelectorSpecialOption[] = [
		{
			value: chatModelFallbackValue,
			dropdownLabel: "Use chat model",
			description: "Use the model selected for the current chat",
		},
	];
	if (
		!isUnsetModelConfigId(form.values.model_config_id) &&
		isLoadingModelConfigs
	) {
		specialOptions.push({
			value: form.values.model_config_id,
			dropdownLabel: "Loading...",
			disabled: true,
		});
	} else if (hasUnavailableSelectedModel) {
		const unavailableLabel = selectedModelConfig
			? `Unavailable: ${getModelDisplayName(selectedModelConfig)}`
			: `Unavailable model (${form.values.model_config_id})`;
		specialOptions.push({
			value: form.values.model_config_id,
			dropdownLabel: unavailableLabel,
			disabled: true,
		});
	}
	const canSave = hasLoadedAdvisorConfig && form.dirty && form.isValid;

	return (
		<AgentSettingLayout
			title="Advisor"
			description="Cap advisor usage per turn and optionally use an override model. The advisor provides strategic guidance to root agent chats. Set limits to 0 for unlimited."
			showSave={canSave}
			isSaving={isSavingAdvisorConfig}
			isSavedVisible={isSavedVisible}
			saveDisabled={isFormDisabled || !canSave}
			onSubmit={form.handleSubmit}
			error={
				isSaveAdvisorConfigError ? (
					<p className="m-0">
						{getErrorMessage(
							saveAdvisorConfigError,
							"Failed to save advisor settings.",
						)}
					</p>
				) : isAdvisorConfigLoadError ? (
					<p className="m-0">Failed to load advisor settings.</p>
				) : undefined
			}
		>
			<CompactIntegerField
				id={maxUsesId}
				name="max_uses_per_run"
				label="Uses / turn"
				ariaLabel="Max uses per turn"
				value={form.values.max_uses_per_run}
				onChange={(value) => void form.setFieldValue("max_uses_per_run", value)}
				onBlur={form.handleBlur}
				error={Boolean(form.errors.max_uses_per_run)}
				disabled={isFormDisabled}
				className="w-[7.5rem]"
			/>
			<CompactIntegerField
				id={maxOutputTokensId}
				name="max_output_tokens"
				label="Max tokens"
				ariaLabel="Max output tokens"
				value={form.values.max_output_tokens}
				onChange={(value) =>
					void form.setFieldValue("max_output_tokens", value)
				}
				onBlur={form.handleBlur}
				error={Boolean(form.errors.max_output_tokens)}
				disabled={isFormDisabled}
				className="w-36"
			/>
			<ModelSelector
				options={modelOptions}
				specialOptions={specialOptions}
				value={selectedModelValue}
				onValueChange={(value) => {
					if (value === chatModelFallbackValue) {
						void form.setValues({
							...form.values,
							model_config_id: "",
							reasoning_effort: "",
						});
						return;
					}
					const option = modelOptions.find((option) => option.id === value);
					let reasoningEffort = "";
					if (option) {
						reasoningEffort =
							pickReasoningEffort(
								"",
								option.reasoningEfforts ?? [],
								option.reasoningEffortDefault,
							) ?? "";
					}
					void form.setValues({
						...form.values,
						model_config_id: value,
						reasoning_effort: reasoningEffort,
					});
				}}
				disabled={isModelSelectDisabled}
				ariaLabel="Advisor model"
				placeholder="Use chat model"
				emptyMessage={
					isLoadingModelConfigs
						? "Loading models..."
						: "No enabled models found."
				}
				className="h-10 w-[22rem] max-w-full justify-between rounded-md border border-border border-solid bg-transparent px-3 text-sm shadow-none"
				contentClassName="min-w-[18rem]"
				reasoningEffort={selectedReasoningEffort}
				onReasoningEffortChange={(value) =>
					void form.setFieldValue("reasoning_effort", value)
				}
			/>
			<Button
				size="lg"
				variant="outline"
				type="button"
				onClick={() => {
					void form.setValues({
						max_uses_per_run: "0",
						max_output_tokens: "0",
						model_config_id: "",
						reasoning_effort: "",
					});
				}}
				disabled={isFormDisabled}
				className="h-10"
			>
				Clear
			</Button>
		</AgentSettingLayout>
	);
};

interface CompactIntegerFieldProps {
	id: string;
	name: string;
	label: string;
	ariaLabel: string;
	value: string;
	onChange: (value: string) => void;
	onBlur: (event: React.FocusEvent<HTMLInputElement>) => void;
	error?: boolean;
	disabled?: boolean;
	className?: string;
}

const CompactIntegerField: FC<CompactIntegerFieldProps> = ({
	id,
	name,
	label,
	ariaLabel,
	value,
	onChange,
	onBlur,
	error,
	disabled,
	className,
}) => {
	return (
		<label
			className={cn(
				"grid h-10 shrink-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-md border border-border border-solid bg-transparent px-3 transition-colors",
				error && "border-border-destructive",
				disabled && "opacity-50",
				className,
			)}
		>
			<input
				id={id}
				type="number"
				name={name}
				min={0}
				step={1}
				inputMode="numeric"
				aria-label={ariaLabel}
				value={value}
				onChange={(event) => onChange(event.currentTarget.value)}
				onBlur={onBlur}
				aria-invalid={error}
				disabled={disabled}
				className="min-w-0 w-full border-none bg-transparent p-0 text-sm font-medium leading-6 text-content-placeholder outline-none disabled:cursor-not-allowed [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none [-moz-appearance:textfield]"
			/>
			<span className="shrink-0 text-xs font-normal leading-[18px] text-content-placeholder">
				{label}
			</span>
		</label>
	);
};
