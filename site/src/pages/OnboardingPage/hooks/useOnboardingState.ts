import { useCallback, useState } from "react";

const STORAGE_KEY = "coder_onboarding";

interface OnboardingState {
	agentsEnabled: boolean;
	currentStep: number;
	providerConfigured: boolean;
	completedAt: string | null;
}

const defaultState: OnboardingState = {
	agentsEnabled: false,
	currentStep: 0,
	providerConfigured: false,
	completedAt: null,
};

function loadState(): OnboardingState {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (raw) {
			return { ...defaultState, ...JSON.parse(raw) };
		}
	} catch {
		// Ignore parse errors and fall through to default.
	}
	return defaultState;
}

function persistState(state: OnboardingState): void {
	localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export interface UseOnboardingStateReturn extends OnboardingState {
	setAgentsEnabled: (enabled: boolean) => void;
	setProviderConfigured: (configured: boolean) => void;
	nextStep: (maxSteps: number) => void;
	prevStep: () => void;
	goToStep: (step: number) => void;
	skipOnboarding: () => void;
	completeOnboarding: () => void;
}

export function useOnboardingState(): UseOnboardingStateReturn {
	const [state, setState] = useState<OnboardingState>(loadState);

	const update = useCallback((partial: Partial<OnboardingState>) => {
		setState((prev) => {
			const next = { ...prev, ...partial };
			persistState(next);
			return next;
		});
	}, []);

	const setAgentsEnabled = useCallback(
		(enabled: boolean) => update({ agentsEnabled: enabled }),
		[update],
	);

	const setProviderConfigured = useCallback(
		(configured: boolean) => update({ providerConfigured: configured }),
		[update],
	);

	const nextStep = useCallback(
		(maxSteps: number) => {
			setState((prev) => {
				const next = {
					...prev,
					currentStep: Math.min(prev.currentStep + 1, maxSteps - 1),
				};
				persistState(next);
				return next;
			});
		},
		[],
	);

	const prevStep = useCallback(() => {
		setState((prev) => {
			const next = {
				...prev,
				currentStep: Math.max(0, prev.currentStep - 1),
			};
			persistState(next);
			return next;
		});
	}, []);

	const goToStep = useCallback(
		(step: number) => update({ currentStep: step }),
		[update],
	);

	const skipOnboarding = useCallback(() => {
		const now = new Date().toISOString();
		update({ completedAt: now });
		localStorage.setItem("onboarding_completed", "true");
	}, [update]);

	const completeOnboarding = useCallback(() => {
		const now = new Date().toISOString();
		update({ completedAt: now, currentStep: 0 });
		localStorage.setItem("onboarding_completed", "true");
	}, [update]);

	return {
		...state,
		setAgentsEnabled,
		setProviderConfigured,
		nextStep,
		prevStep,
		goToStep,
		skipOnboarding,
		completeOnboarding,
	};
}
