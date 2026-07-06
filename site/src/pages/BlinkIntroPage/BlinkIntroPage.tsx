import { type FC, useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { Blink } from "#/components/Blink/Blink";
import { BlinkProvider, useBlinkContext } from "#/components/Blink/BlinkProvider";
import { Button } from "#/components/Button/Button";
import { ProductLogo } from "#/components/Icons/ProductLogo";
import { useAuthContext } from "#/contexts/auth/AuthProvider";
import { Loader } from "#/components/Loader/Loader";
import { pageTitle } from "#/utils/page";

/**
 * Shown once after first-user setup when Blink was enabled.
 * Introduces the floating assistant and nudges the user to try it.
 * Dismissable at any time.
 */
const BlinkIntroContent: FC = () => {
	const navigate = useNavigate();
	const { toggle, open } = useBlinkContext();
	const [pointed, setPointed] = useState(false);

	// Once the user opens Blink from this page, mark the intro as done.
	useEffect(() => {
		if (open && pointed) {
			localStorage.setItem("blink_intro_completed", "true");
		}
	}, [open, pointed]);

	const handleTryBlink = () => {
		setPointed(true);
		toggle();
	};

	const handleSkip = () => {
		localStorage.setItem("blink_intro_completed", "true");
		void navigate("/templates");
	};

	return (
		<>
			<title>{pageTitle("Meet Blink")}</title>
			<div className="grow basis-0 min-h-screen flex justify-center items-center py-12">
				<div className="flex flex-col items-center w-full max-w-[480px] px-4 text-center gap-8">
					<header className="flex flex-col items-center gap-4">
						<ProductLogo className="h-10" />
						<h1 className="text-3xl font-semibold m-0">
							Meet Blink
						</h1>
						<p className="text-sm text-content-secondary m-0 leading-relaxed max-w-sm">
							Blink is your Coder assistant. It lives in the
							bottom-right corner of your dashboard and can help
							you set up templates, create workspaces, manage
							users, and answer questions about your deployment.
						</p>
					</header>

					{/* Visual pointer toward the floating button */}
					<div className="flex flex-col items-center gap-3">
						<p className="text-sm text-content-primary m-0 font-medium">
							Click the button in the bottom-right corner to get
							started, or use the button below.
						</p>
						<div className="flex items-center gap-2 text-content-secondary">
							<svg
								className="w-6 h-6 animate-bounce"
								fill="none"
								stroke="currentColor"
								viewBox="0 0 24 24"
							>
								<path
									strokeLinecap="round"
									strokeLinejoin="round"
									strokeWidth={2}
									d="M19 14l-7 7m0 0l-7-7m7 7V3"
								/>
							</svg>
						</div>
					</div>

					<div className="flex gap-3">
						<Button variant="outline" onClick={handleSkip}>
							Skip to dashboard
						</Button>
						<Button onClick={handleTryBlink}>
							Try Blink
						</Button>
					</div>
				</div>
			</div>

			{/* Blink floats here so user can interact with it */}
			<Blink />
		</>
	);
};

export const BlinkIntroPage: FC = () => {
	const { isLoading, isSignedIn } = useAuthContext();
	const navigate = useNavigate();

	// Guard: must be signed in.
	useEffect(() => {
		if (!isLoading && !isSignedIn) {
			void navigate("/login", { replace: true });
		}
	}, [isLoading, isSignedIn, navigate]);

	// Guard: only show once.
	useEffect(() => {
		if (localStorage.getItem("blink_intro_completed") === "true") {
			void navigate("/templates", { replace: true });
		}
	}, [navigate]);

	if (isLoading) {
		return <Loader fullscreen />;
	}

	return (
		<BlinkProvider forceEnabled>
			<BlinkIntroContent />
		</BlinkProvider>
	);
};
