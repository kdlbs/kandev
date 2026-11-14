import { test, expect } from "../../fixtures/test-base";
import {
  checkAdvancedHandoff,
  checkTouchPicker,
  cleanupAdvancedServers,
  expectCreatedSelection,
  expectContainedPicker,
  seedAdvancedSession,
} from "./new-session-advanced-helpers";

// @covers AC-UI-NEW-SESSION-ADVANCED-001.1/3/4, AC-UI-NEW-SESSION-ADVANCED-002.1/2/3/4
test.describe("New session Advanced settings on phone", () => {
  test.setTimeout(120_000);
  test("preserves the handoff profile and explicit blank context", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await checkAdvancedHandoff(testPage, apiClient, seedData, true);
  });
  test("contains long translated copy and reachable choices at 320px", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const { session, definitions } = await seedAdvancedSession(testPage, apiClient, seedData);
    await testPage
      .context()
      .addCookies([{ name: "kandev_locale", value: "pseudo", url: testPage.url() }]);
    await testPage.setViewportSize({ width: 320, height: 800 });
    await testPage.reload();
    await expect(testPage.locator("html")).toHaveAttribute("lang", "pseudo");
    await session.openMobileNewSessionDialog();
    const controls = session.newSessionDialogPage;
    await controls.advancedTrigger().tap();
    await controls.mcpTrigger().tap();
    const picker = controls.mcpPicker();
    await expectContainedPicker(testPage, picker);
    const row = picker.getByTestId(`mcp-choice-${definitions[1].id}`);
    await row.scrollIntoViewIfNeeded();
    expect(
      await row.evaluate((el) => {
        const box = el.getBoundingClientRect();
        return el.contains(
          document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
        );
      }),
    ).toBe(true);
    await row.tap();
    await expect(row.getByRole("checkbox")).toBeChecked();
    await testPage.screenshot({ path: testInfo.outputPath("phone-pseudo-picker.png") });
    const close = picker.locator('[data-slot="drawer-close"]');
    await close.tap();
    await expect(controls.mcpTrigger()).toBeFocused();
  });
  test.afterEach(async ({ apiClient, seedData }) => {
    await cleanupAdvancedServers(apiClient, seedData.workspaceId);
  });
  test("uses a bounded sheet and creates the agent with its selected additions", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const { task, session, definitions } = await seedAdvancedSession(testPage, apiClient, seedData);
    await session.openMobileNewSessionDialog();
    await checkTouchPicker(testPage, session, definitions[1].id);
    await session.newSessionDialogPage.mcpTrigger().tap();
    await expectContainedPicker(testPage, session.newSessionDialogPage.mcpPicker());
    await testPage.screenshot({ path: testInfo.outputPath("phone-picker.png") });
    await session.newSessionDialogPage
      .mcpPicker()
      .getByRole("button", { name: "Close", exact: true })
      .tap();
    await expect(session.newSessionDialogPage.mcpPicker()).not.toBeVisible();
    await testPage.screenshot({ path: testInfo.outputPath("phone-advanced.png") });
    await session.newSessionStartButton().tap();
    await expect(session.sessionLaunchDialog()).not.toBeVisible();
    await expectCreatedSelection(apiClient, seedData, task.id, task.session_id!, definitions[1].id);
  });
});
