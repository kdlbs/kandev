import { test, expect } from "../../fixtures/test-base";
import {
  SETTINGS_APPEARANCE_PATH as APPEARANCE_PATH,
  SETTINGS_TAKEOVER_TESTID as TAKEOVER,
  setSettingsMenuMode as setMenuMode,
} from "../../helpers/settings-menu";

// The settings menu shape is a per-device preference (localStorage), so these
// specs assert against the sidebar and a reload rather than the API — there is
// no server state to read back, which is the point of the setting.
test.describe("Settings menu modes", () => {
  test("previews the chosen shape in the sidebar before it is saved", async ({ testPage }) => {
    await testPage.goto(APPEARANCE_PATH);
    const takeover = testPage.getByTestId(TAKEOVER);
    // Flat is the default: the Workspaces row is a link and nothing more.
    await expect(takeover.getByRole("button", { name: /Expand Workspaces/ })).toHaveCount(0);

    await testPage.getByTestId("settings-menu-mode-accordion").click();

    // Previewed immediately — the sidebar is on screen beside the control.
    await expect(takeover.getByRole("button", { name: /Expand Workspaces/ })).toBeVisible();
    await expect(testPage.getByTestId("settings-menu-mode-card")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );
  });

  test("puts the menu back when the change is discarded", async ({ testPage }) => {
    await testPage.goto(APPEARANCE_PATH);
    const takeover = testPage.getByTestId(TAKEOVER);

    await testPage.getByTestId("settings-menu-mode-persistent").click();
    await expect(takeover.getByRole("button", { name: /Expand Workspaces/ })).toBeVisible();

    await takeover.getByRole("link", { name: "Prompts", exact: true }).first().click();
    const dialog = testPage.getByRole("alertdialog", { name: "Save changes before leaving?" });
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Discard and leave" }).click();

    await expect(takeover.getByRole("button", { name: /Expand Workspaces/ })).toHaveCount(0);
  });

  test("keeps one branch open at a time in accordion mode", async ({ testPage, seedData }) => {
    await setMenuMode(testPage, "accordion");
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
    const takeover = testPage.getByTestId(TAKEOVER);

    // The route opens its own branch; the others stay shut.
    await expect(takeover.getByRole("button", { name: /Collapse Workspaces/ })).toBeVisible();
    await expect(takeover.getByRole("link", { name: "Integrations", exact: true })).toBeVisible();
    await expect(takeover.getByRole("button", { name: /Collapse Agents/ })).toHaveCount(0);

    await takeover.getByRole("button", { name: /Expand Agents/ }).click();

    await expect(takeover.getByRole("button", { name: /Collapse Agents/ })).toBeVisible();
    await expect(takeover.getByRole("link", { name: "Integrations", exact: true })).toHaveCount(0);
  });

  test("keeps several branches open, and across a reload, in persistent mode", async ({
    testPage,
    seedData,
  }) => {
    await setMenuMode(testPage, "persistent");
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}`);
    const takeover = testPage.getByTestId(TAKEOVER);

    await expect(takeover.getByRole("button", { name: /Collapse Workspaces/ })).toBeVisible();
    await takeover.getByRole("button", { name: /Expand Agents/ }).click();

    // Both stay open — the difference from accordion.
    await expect(takeover.getByRole("button", { name: /Collapse Workspaces/ })).toBeVisible();
    await expect(takeover.getByRole("button", { name: /Collapse Agents/ })).toBeVisible();

    await testPage.reload();

    await expect(takeover.getByRole("button", { name: /Collapse Workspaces/ })).toBeVisible();
    await expect(takeover.getByRole("button", { name: /Collapse Agents/ })).toBeVisible();
  });

  test("drills a workspace down to a single integration", async ({ testPage, seedData }) => {
    await setMenuMode(testPage, "accordion");
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/integrations/github`);
    const takeover = testPage.getByTestId(TAKEOVER);

    // Settings › Workspaces › <workspace> › Integrations › GitHub, the deepest
    // branch the menu has — and the same chain the page's breadcrumb renders.
    await expect(takeover.getByRole("link", { name: "GitHub", exact: true })).toBeVisible();

    // Only the page you are on is marked, never the rows that merely contain it.
    const active = takeover.locator("[data-active='true']");
    await expect(active).toHaveCount(1);
    await expect(active).toHaveAccessibleName("GitHub");
  });
});
