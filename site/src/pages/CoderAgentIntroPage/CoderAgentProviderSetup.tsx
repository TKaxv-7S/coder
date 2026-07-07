import { type CSSProperties, type FC, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { createAIProviderMutation } from "#/api/queries/aiProviders";
import { createChatModelConfig } from "#/api/queries/chats";
import type { AIProviderType } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { getKnownModelsForProvider } from "#/pages/AgentsPage/components/ChatModelAdminPanel/knownModels";
import { cn } from "#/utils/cn";

interface CoderAgentProviderSetupProps {
	onComplete: () => void;
	onSkip: () => void;
}

interface ModelOption {
	identifier: string;
	displayName: string;
	contextLimit?: number;
}

interface ProviderOption {
	type: AIProviderType;
	name: string;
	defaultBaseUrl: string;
	/** Fallback models for providers without a known-models catalog. */
	fallbackModels?: ModelOption[];
}

const providers: ProviderOption[] = [
	{
		type: "anthropic",
		name: "Anthropic",
		defaultBaseUrl: "https://api.anthropic.com",
	},
	{
		type: "openai",
		name: "OpenAI",
		defaultBaseUrl: "https://api.openai.com/v1",
	},
	{
		type: "google",
		name: "Google",
		defaultBaseUrl: "https://generativelanguage.googleapis.com/v1beta",
		fallbackModels: [
			{ identifier: "gemini-2.5-pro", displayName: "Gemini 2.5 Pro" },
			{ identifier: "gemini-2.5-flash", displayName: "Gemini 2.5 Flash" },
		],
	},
];

// Keep the intro focused: show a handful of current models rather
// than the full catalog.
const maxModelOptions = 5;

/**
 * Models for a provider, sourced from the shared known-models catalog
 * used by the AI admin settings, with a static fallback for providers
 * the catalog doesn't cover.
 */
function modelsForProvider(provider: ProviderOption): ModelOption[] {
	const known = getKnownModelsForProvider(provider.type);
	if (known.length > 0) {
		return known.slice(0, maxModelOptions).map((model) => ({
			identifier: model.modelIdentifier,
			displayName: model.displayName,
			contextLimit: model.contextLimit,
		}));
	}
	return provider.fallbackModels ?? [];
}

/**
 * Provider setup shown as the first step of the Coder Agent intro flow.
 * Creates an AI provider and a default model config so the Coder Agent
 * has a working backend before the user first talks to it.
 */
export const CoderAgentProviderSetup: FC<CoderAgentProviderSetupProps> = ({
	onComplete,
	onSkip,
}) => {
	const queryClient = useQueryClient();
	const createProvider = useMutation(createAIProviderMutation(queryClient));
	const createModelConfig = useMutation(createChatModelConfig(queryClient));

	const [selectedProvider, setSelectedProvider] =
		useState<ProviderOption | null>(null);
	const [apiKey, setApiKey] = useState("");
	const [baseUrl, setBaseUrl] = useState("");
	const [selectedModel, setSelectedModel] = useState<ModelOption | null>(null);

	const models = selectedProvider ? modelsForProvider(selectedProvider) : [];
	const isPending = createProvider.isPending || createModelConfig.isPending;
	const error = createProvider.error || createModelConfig.error;

	const handleProviderSelect = (provider: ProviderOption) => {
		setSelectedProvider(provider);
		setBaseUrl(provider.defaultBaseUrl);
		setSelectedModel(modelsForProvider(provider)[0] ?? null);
	};

	const handleSave = () => {
		// Pasted keys frequently carry surrounding whitespace or a
		// trailing newline, which the API rejects. Trim before sending.
		const trimmedKey = apiKey.trim();
		if (!selectedProvider || !trimmedKey || !selectedModel) {
			return;
		}
		createProvider.mutate(
			{
				type: selectedProvider.type,
				name: selectedProvider.type,
				display_name: selectedProvider.name,
				enabled: true,
				base_url: baseUrl.trim(),
				api_keys: [trimmedKey],
			},
			{
				onSuccess: (provider) => {
					createModelConfig.mutate(
						{
							ai_provider_id: provider.id,
							model: selectedModel.identifier,
							display_name: selectedModel.displayName,
							enabled: true,
							is_default: true,
							context_limit: selectedModel.contextLimit,
						},
						{
							onSuccess: onComplete,
						},
					);
				},
			},
		);
	};

	return (
		<div className="flex flex-col gap-6 w-full">
			<header className="text-center">
				<h2 className="text-2xl font-semibold m-0">Set up your Coder Agent</h2>
				<p className="text-sm text-content-secondary mt-2 mb-0">
					The Coder Agent needs an AI provider to work. Connect one now so it's
					ready when you are.
				</p>
			</header>

			{/* Provider cards */}
			<div className="grid grid-cols-3 gap-3">
				{providers.map((provider) => (
					<button
						key={provider.type}
						type="button"
						onClick={() => handleProviderSelect(provider)}
						className={cn(
							"flex flex-col items-center gap-2 p-4 rounded-lg border border-solid cursor-pointer transition-colors text-center bg-transparent",
							selectedProvider?.type === provider.type
								? "border-content-link bg-surface-secondary"
								: "border-border hover:border-border-secondary",
						)}
					>
						<span className="text-sm font-medium text-content-primary">
							{provider.name}
						</span>
					</button>
				))}
			</div>

			{selectedProvider && (
				<div className="flex flex-col gap-4">
					{/* API Key */}
					<div className="flex flex-col gap-2">
						<Label htmlFor="agent-api-key">API Key</Label>
						<Input
							id="agent-api-key"
							type="text"
							value={apiKey}
							onChange={(e) => setApiKey(e.target.value)}
							placeholder={`Enter your ${selectedProvider.name} API key`}
							style={{ WebkitTextSecurity: "disc" } as CSSProperties}
						/>
					</div>

					{/* Base URL */}
					<div className="flex flex-col gap-2">
						<Label htmlFor="agent-base-url">Base URL (optional)</Label>
						<Input
							id="agent-base-url"
							type="text"
							value={baseUrl}
							onChange={(e) => setBaseUrl(e.target.value)}
							placeholder="https://..."
						/>
					</div>

					{/* Model selector */}
					<div className="flex flex-col gap-2">
						<Label>Model</Label>
						<div className="flex flex-col gap-1">
							{models.map((model) => (
								<label
									key={model.identifier}
									htmlFor={`agent-model-${model.identifier}`}
									className={cn(
										"flex items-center gap-3 px-3 py-2 rounded-md cursor-pointer transition-colors",
										selectedModel?.identifier === model.identifier
											? "bg-surface-secondary"
											: "hover:bg-surface-secondary",
									)}
								>
									<input
										type="radio"
										id={`agent-model-${model.identifier}`}
										name="agent-model"
										value={model.identifier}
										checked={selectedModel?.identifier === model.identifier}
										onChange={() => setSelectedModel(model)}
										className="accent-content-link"
									/>
									<span className="flex flex-col">
										<span className="text-sm">{model.displayName}</span>
										<span className="text-xs text-content-secondary">
											{model.identifier}
										</span>
									</span>
								</label>
							))}
						</div>
					</div>

					{Boolean(error) && (
						<p className="text-sm text-content-destructive m-0">
							{getErrorMessage(error, "Failed to save the provider or model.")}
						</p>
					)}
				</div>
			)}

			<div className="flex justify-between items-center pt-2">
				<Button variant="subtle" onClick={onSkip}>
					Skip for now
				</Button>
				{selectedProvider ? (
					<Button disabled={!apiKey.trim() || isPending} onClick={handleSave}>
						<Spinner loading={isPending} />
						Save &amp; Continue
					</Button>
				) : (
					<span className="text-xs text-content-secondary">
						Select a provider above to continue
					</span>
				)}
			</div>
		</div>
	);
};
