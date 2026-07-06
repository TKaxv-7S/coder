import type { FC } from "react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import type { UseOnboardingStateReturn } from "./hooks/useOnboardingState";
import { AgentChatStep } from "./steps/AgentChatStep";
import { ConfigureProviderStep } from "./steps/ConfigureProviderStep";
import { SummaryStep } from "./steps/SummaryStep";
import { WelcomeStep } from "./steps/WelcomeStep";

interface OnboardingPageViewProps {
	state: UseOnboardingStateReturn;
	onComplete: () => void;
}

interface StepDef {
	key: string;
	label: string;
	/** Use the wider layout for this step. */
	wide?: boolean;
}

function getSteps(agentsEnabled: boolean): StepDef[] {
	const steps: StepDef[] = [{ key: "welcome", label: "Welcome" }];
	if (agentsEnabled) {
		steps.push({ key: "provider", label: "AI Provider" });
		steps.push({ key: "blink", label: "Blink", wide: true });
	}
	steps.push({ key: "summary", label: "Summary" });
	return steps;
}

export const OnboardingPageView: FC<OnboardingPageViewProps> = ({
	state,
	onComplete,
}) => {
	const steps = getSteps(state.agentsEnabled);
	const safeIndex = Math.min(state.currentStep, steps.length - 1);
	const currentStepDef = steps[safeIndex];
	const isFirst = safeIndex === 0;
	const isLast = safeIndex === steps.length - 1;

	const advance = () => state.nextStep(steps.length);

	const renderStep = () => {
		switch (currentStepDef.key) {
			case "welcome":
				return (
					<WelcomeStep
						agentsEnabled={state.agentsEnabled}
						onAgentsEnabledChange={state.setAgentsEnabled}
						onContinue={advance}
					/>
				);
			case "provider":
				return (
					<ConfigureProviderStep
						onComplete={advance}
						onSkip={advance}
						onProviderConfigured={() => state.setProviderConfigured(true)}
					/>
				);
			case "blink":
				return <AgentChatStep onFinish={advance} />;
			case "summary":
				return (
					<SummaryStep
						agentsEnabled={state.agentsEnabled}
						providerConfigured={state.providerConfigured}
						onComplete={onComplete}
					/>
				);
			default:
				return null;
		}
	};

	return (
		<div className="grow basis-0 min-h-screen flex flex-col items-center py-12">
			{/* Progress dots */}
			<div className="flex items-center gap-2 mb-8">
				{steps.map((step, i) => (
					<div key={step.key} className="flex items-center gap-2">
						<div
							className={cn(
								"w-2.5 h-2.5 rounded-full transition-colors",
								i === safeIndex
									? "bg-content-link"
									: i < safeIndex
										? "bg-surface-invert-primary"
										: "bg-surface-quaternary",
							)}
							title={step.label}
						/>
						{i < steps.length - 1 && (
							<div
								className={cn(
									"w-8 h-0.5 transition-colors",
									i < safeIndex
										? "bg-surface-invert-primary"
										: "bg-surface-quaternary",
								)}
							/>
						)}
					</div>
				))}
			</div>

			{/* Step content */}
			<div
				className={cn(
					"w-full px-4",
					currentStepDef.wide ? "max-w-[800px]" : "max-w-[500px]",
				)}
			>
				{renderStep()}
			</div>

			{/* Navigation (hidden for welcome and summary since they have their own buttons) */}
			{currentStepDef.key !== "welcome" &&
				currentStepDef.key !== "summary" && (
					<div
						className={cn(
							"w-full px-4 mt-8 flex justify-between",
							currentStepDef.wide ? "max-w-[800px]" : "max-w-[500px]",
						)}
					>
						{!isFirst ? (
							<Button variant="outline" onClick={state.prevStep}>
								Back
							</Button>
						) : (
							<div />
						)}
						{!isLast && (
							<Button variant="subtle" onClick={advance}>
								Skip
							</Button>
						)}
					</div>
				)}
		</div>
	);
};
