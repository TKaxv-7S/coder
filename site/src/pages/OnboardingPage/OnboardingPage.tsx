import type { FC } from "react";
import { Navigate, useNavigate } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { pageTitle } from "#/utils/page";
import { OnboardingPageView } from "./OnboardingPageView";
import { useOnboardingState } from "./hooks/useOnboardingState";

export const OnboardingPage: FC = () => {
	const { isLoading, isSignedIn } = useAuthContext();
	const navigate = useNavigate();
	const state = useOnboardingState();

	if (isLoading) {
		return <Loader fullscreen />;
	}

	if (!isSignedIn) {
		return <Navigate to="/login" replace />;
	}

	// If onboarding was already completed, go to the dashboard.
	if (localStorage.getItem("onboarding_completed") === "true") {
		return <Navigate to="/" state={{ isRedirect: true }} replace />;
	}

	const handleComplete = () => {
		state.completeOnboarding();
		void navigate("/");
	};

	return (
		<>
			<title>{pageTitle("Onboarding")}</title>
			<OnboardingPageView state={state} onComplete={handleComplete} />
		</>
	);
};
