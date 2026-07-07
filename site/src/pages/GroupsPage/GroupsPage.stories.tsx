import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { getGroupsByOrganizationQueryKey } from "#/api/queries/groups";
import { organizationPermissionChecks } from "#/modules/permissions/organizations";
import {
	MockDefaultOrganization,
	MockGroup,
	MockOrganizationPermissions,
} from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import GroupsPage from "./GroupsPage";
import GroupsPageProvider from "./GroupsPageProvider";

const organizationPermissionsKey = getAuthorizationKey({
	checks: Object.fromEntries(
		Object.entries(
			organizationPermissionChecks(MockDefaultOrganization.id),
		).map(([key, value]) => [`${MockDefaultOrganization.id}.${key}`, value]),
	),
});

const meta: Meta<typeof GroupsPageProvider> = {
	title: "pages/OrganizationGroupsPage/GroupsPage",
	component: GroupsPageProvider,
	decorators: [withDashboardProvider],
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				pathParams: {
					organization: MockDefaultOrganization.name,
				},
			},
			routing: reactRouterOutlet(
				{ path: "/organizations/:organization/groups" },
				<GroupsPage />,
			),
		}),
	},
};

export default meta;
type Story = StoryObj<typeof GroupsPageProvider>;

export const PremiumDisabled: Story = {
	parameters: {
		queries: [],
	},
	beforeEach: () => {
		const getGroupsByOrganization = spyOn(
			API,
			"getGroupsByOrganization",
		).mockResolvedValue([MockGroup]);
		const checkAuthorization = spyOn(
			API,
			"checkAuthorization",
		).mockResolvedValue({});

		return () => {
			getGroupsByOrganization.mockRestore();
			checkAuthorization.mockRestore();
		};
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByRole("heading", { name: "Groups", level: 1 });
		expect(
			canvas.getByText(/You need a Premium license to use this feature/i),
		).toBeVisible();
		expect(API.getGroupsByOrganization).not.toHaveBeenCalled();
		expect(API.checkAuthorization).not.toHaveBeenCalled();
	},
};

export const PremiumEnabled: Story = {
	parameters: {
		features: ["template_rbac"],
		queries: [
			{
				key: organizationPermissionsKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
				},
			},
			{
				key: getGroupsByOrganizationQueryKey(MockDefaultOrganization.name),
				data: [MockGroup],
			},
		],
	},
};
