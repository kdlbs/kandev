import { expect, type Page } from "@playwright/test";
import path from "node:path";
import { backendFixture as test, type BackendContext } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";

const ADMIN = {
  email: "owner@workspace-access.example.test",
  password: "workspace-test-admin123",
  displayName: "Workspace owner",
};
const AUTH_HEADING = "Authentication & users";

async function openOverview(page: Page, backend: BackendContext) {
  const workspace = await page.request.post(`${backend.baseUrl}/api/v1/workspaces`, {
    data: { name: "Workspace access test" },
  });
  expect(workspace.ok(), await workspace.text()).toBeTruthy();
  const { id } = (await workspace.json()) as { id: string };
  await page.addInitScript(() => {
    window.localStorage.setItem("kandev.onboarding.completed", "true");
  });
  await page.goto(`${backend.frontendUrl}/settings/workspaces/${id}`);
  await expect(page.getByTestId("workspace-idle-policy-card")).toBeVisible();
  return id;
}

async function createUnit(page: Page, backend: BackendContext) {
  const response = await page.request.get(`${backend.baseUrl}/api/v1/units`);
  expect(response.ok(), await response.text()).toBeTruthy();
  const { units } = (await response.json()) as { units: Array<{ id: string; kind: string }> };
  const root = units.find((unit) => unit.kind === "root");
  expect(root).toBeDefined();
  const created = await page.request.post(`${backend.baseUrl}/api/v1/units`, {
    data: { name: "Runtime", parent_id: root!.id },
  });
  expect(created.ok(), await created.text()).toBeTruthy();
  return ((await created.json()) as { id: string }).id;
}

export function registerWorkspaceAccessSettingsTests(mobile: boolean) {
  test.afterEach(async ({ backend }) => {
    await backend.restart();
  });

  // @covers AC-WORKSPACES-ORG-UNITS-005.6
  test("omits the workspace access section when authentication is off", async ({
    backend,
    page,
  }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "false",
      KANDEV_FEATURES_MULTI_TENANCY: "false",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "workspace-access-auth-off.db"),
    });
    await openOverview(page, backend);

    await expect(page.getByTestId("workspace-settings-delete-button")).toBeVisible();
    await expect(page.getByRole("heading", { name: AUTH_HEADING })).toHaveCount(0);
    await expect
      .soft(page.getByTestId("workspace-placement-card"))
      .toHaveCount(0, { timeout: 500 });
    await expect(page.getByTestId("workspace-team-access-card")).toHaveCount(0, { timeout: 500 });
  });

  // @covers AC-WORKSPACES-ORG-UNITS-005.7 AC-WORKSPACES-ORG-UNITS-005.8
  test("explains both ways to share a workspace under Authentication & users", async ({
    backend,
    context,
    page,
  }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_FEATURES_MULTI_TENANCY: "false",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "workspace-access-auth-on.db"),
    });
    await setupAdmin(context, backend.baseUrl, ADMIN);
    await login(context, backend.baseUrl, ADMIN);
    const unitId = await createUnit(page, backend);
    const workspaceId = await openOverview(page, backend);

    const group = page.getByRole("region", { name: AUTH_HEADING });
    await expect(group.getByRole("heading", { name: AUTH_HEADING })).toBeVisible();
    await expect(
      group.getByText("Manage access to this workspace for signed-in users."),
    ).toBeVisible();
    const placement = group.getByTestId("workspace-placement-card");
    await expect(placement.getByText(/parent units can open this workspace/)).toBeVisible();
    await expect(placement.getByText(/keep the role you gave them/)).toBeVisible();
    await expect(
      group.getByTestId("workspace-team-access-card").getByText(/highest role applies/),
    ).toBeVisible();

    const picker = placement.getByRole("combobox", { name: "Organization unit" });
    await expect(picker).toBeEnabled();
    if (mobile) {
      const box = await picker.boundingBox();
      expect(box?.height).toBeGreaterThanOrEqual(44);
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
        page.viewportSize()!.width,
      );
      await picker.tap();
    } else {
      await picker.click();
    }

    const option = page.getByRole("option", { name: "Runtime" });
    const move = page.waitForResponse(
      (response) =>
        response.url() === `${backend.baseUrl}/api/v1/workspaces/${workspaceId}` &&
        response.request().method() === "PATCH",
    );
    await (mobile ? option.tap() : option.click());
    const response = await move;
    expect(response.ok(), await response.text()).toBeTruthy();
    expect((await response.json()).unit_id).toBe(unitId);
    await expect(picker).toHaveText("Runtime");

    await page.reload();
    const restoredPlacement = page
      .getByRole("region", { name: AUTH_HEADING })
      .getByTestId("workspace-placement-card");
    await expect(restoredPlacement.getByRole("combobox", { name: "Organization unit" })).toHaveText(
      "Runtime",
    );
  });
}
