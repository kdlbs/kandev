import { screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { AuthSliceState } from "@/lib/state/slices/auth/types";
import {
  cleanupWorkspaceFixture,
  mountWorkspace,
  TARGET_ID,
} from "./workspace-edit-save.test-helpers";

const AUTH_HEADING = "Authentication & users";
const ACCESS_READS = [
  "/api/v1/units",
  `/api/v1/workspaces/${TARGET_ID}/members`,
  "/api/v1/users/directory",
];
const SIGNED_IN = {
  mode: "enabled",
  authenticated: true,
  user: {
    id: "original-owner",
    email: "owner@example.test",
    display_name: "Workspace owner",
    role: "admin",
    status: "active",
  },
} satisfies AuthSliceState["auth"];

afterEach(cleanupWorkspaceFixture);

// @covers AC-WORKSPACES-ORG-UNITS-005.6
it("hidesWorkspaceAccessSettingsWhenAuthenticationIsDisabled", async () => {
  const fixture = await mountWorkspace(
    { scopes: ["workspace.read", "workspace.manage", "member.manage"] },
    {
      mode: "disabled",
      authenticated: true,
      user: { ...SIGNED_IN.user, id: "default-user" },
    },
  );

  expect(screen.queryByTestId("workspace-idle-policy-card")).not.toBeNull();
  expect(screen.queryByTestId("workspace-settings-delete-button")).not.toBeNull();
  expect.soft(screen.queryByRole("heading", { name: AUTH_HEADING })).toBeNull();
  expect.soft(screen.queryByTestId("workspace-placement-card")).toBeNull();
  expect.soft(screen.queryByTestId("workspace-team-access-card")).toBeNull();
  expect(fixture.reads).toEqual([]);
});

// @covers AC-WORKSPACES-ORG-UNITS-005.7 AC-WORKSPACES-ORG-UNITS-005.8
it("groups unit placement and individual roles under authentication settings", async () => {
  const fixture = await mountWorkspace(
    { scopes: ["workspace.read", "workspace.manage", "member.manage"] },
    SIGNED_IN,
  );

  const group = within(screen.getByRole("region", { name: AUTH_HEADING }));
  expect(group.queryByRole("heading", { name: AUTH_HEADING })).not.toBeNull();
  const placement = within(group.getByTestId("workspace-placement-card"));
  expect(placement.queryByText("Organization unit")).not.toBeNull();
  expect(placement.queryByText(/parent units can open this workspace/)).not.toBeNull();
  expect(placement.queryByText(/keep the role you gave them/)).not.toBeNull();
  expect(placement.getByRole("combobox", { name: "Organization unit" })).toHaveProperty(
    "disabled",
    false,
  );
  const members = within(group.getByTestId("workspace-team-access-card"));
  expect(members.queryByText(/their highest role applies/)).not.toBeNull();
  expect(members.queryByText("Add a colleague")).not.toBeNull();
  expect(fixture.reads).toEqual(expect.arrayContaining(ACCESS_READS));
});

// @covers AC-WORKSPACES-ORG-UNITS-005.7
it("shows signed-in readers both cards with read-only controls", async () => {
  await mountWorkspace({ scopes: ["workspace.read"] }, SIGNED_IN);

  const group = within(screen.getByRole("region", { name: AUTH_HEADING }));
  expect(group.getByRole("combobox", { name: "Organization unit" })).toHaveProperty(
    "disabled",
    true,
  );
  expect(group.queryByTestId("workspace-team-access-card")).not.toBeNull();
  expect(group.queryByText("Add a colleague")).toBeNull();
});
