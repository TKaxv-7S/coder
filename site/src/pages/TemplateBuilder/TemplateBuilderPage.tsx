import { type FC, useEffect } from "react";
import { useMutation, useQuery } from "react-query";
import { Navigate, useNavigate } from "react-router";
import { deploymentConfig } from "#/api/queries/deployment";
import {
	createTemplateFromBuilder,
	recordTemplateBuilderSession,
	templateBuilderBases,
} from "#/api/queries/templateBuilder";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { linkToTemplate, useLinks } from "#/modules/navigation";
import { pageTitle } from "#/utils/page";
import { TemplateBuilderPageView } from "./TemplateBuilderPageView";
import type { TemplateBuilderWizardState } from "./wizardState";
import { toCreateTemplateRequest } from "./wizardState";

const TemplateBuilderPage: FC = () => {
	const navigate = useNavigate();
	const getLink = useLinks();
	const { permissions } = useAuthenticated();
	const { data, error, isLoading } = useQuery(deploymentConfig());
	const createMutation = useMutation(createTemplateFromBuilder());
	const sessionMutation = useMutation(recordTemplateBuilderSession());

	const builderDisabled = data?.config?.template_builder?.disabled ?? false;
	const wizardReady =
		!builderDisabled && !isLoading && permissions.createTemplates;

	// Report wizard_entry once the builder is ready and accessible.
	const reportSession = sessionMutation.mutate;
	useEffect(() => {
		if (!wizardReady) {
			return;
		}
		reportSession({ event_type: "wizard_entry" });
	}, [wizardReady, reportSession]);

	const basesQuery = useQuery({
		...templateBuilderBases(),
		enabled: wizardReady,
	});

	if (isLoading) {
		return <Loader />;
	}

	if (!permissions.createTemplates) {
		return <Navigate to="/templates" replace />;
	}

	if (builderDisabled) {
		return <Navigate to="/templates/new" replace />;
	}

	const handleCreate = (state: TemplateBuilderWizardState) => {
		const req = toCreateTemplateRequest(state);
		const durationSeconds = (Date.now() - state.enteredAt) / 1000;

		createMutation.mutate(req, {
			onSuccess: (resp) => {
				sessionMutation.mutate({
					event_type: "compose_completion",
					base_template_id: state.baseTemplateId ?? undefined,
					module_ids: state.modules.map((m) => m.id),
					duration_seconds: durationSeconds,
					success: true,
				});
				const t = resp.template;
				navigate(
					`${getLink(linkToTemplate(t.organization_name, t.name))}/files`,
					{ state: { justCreated: true } },
				);
			},
			onError: () => {
				sessionMutation.mutate({
					event_type: "compose_completion",
					base_template_id: state.baseTemplateId ?? undefined,
					module_ids: state.modules.map((m) => m.id),
					duration_seconds: durationSeconds,
					success: false,
				});
			},
		});
	};

	return (
		<>
			<title>{pageTitle("Create Template")}</title>
			<TemplateBuilderPageView
				error={error}
				basesData={basesQuery.data}
				onCreateTemplate={handleCreate}
				createError={createMutation.error}
				isCreating={createMutation.isPending}
				onClearCreateError={() => createMutation.reset()}
			/>
		</>
	);
};

export default TemplateBuilderPage;
