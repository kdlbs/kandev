import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  checkAdvancedHandoff,
  checkTouchPicker,
  cleanupAdvancedServers,
  expectContainedPicker,
  expectCreatedSelection,
  PROMPT,
  seedAdvancedSession,
} from "./new-session-advanced-helpers";

// @covers AC-UI-NEW-SESSION-ADVANCED-001.1/2/3/4, AC-UI-NEW-SESSION-ADVANCED-002.1/2/3/4
test.describe("New session Advanced settings", () => {
  test.setTimeout(120_000);
  test("preserves the handoff profile and explicit blank context", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await checkAdvancedHandoff(testPage, apiClient, seedData, false);
  });
  test.afterEach(async ({ apiClient, seedData }) => {
    await cleanupAdvancedServers(apiClient, seedData.workspaceId);
  });

  test("preserves the draft and submits only explicit additions through a bounded keyboard picker", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const { task, session, definitions } = await seedAdvancedSession(testPage, apiClient, seedData);
    await session.openNewSessionDialog();
    const dialog = session.sessionLaunchDialog();
    const advanced = session.newSessionDialogPage.advancedTrigger();
    const trigger = session.newSessionDialogPage.mcpTrigger();
    const picker = session.newSessionDialogPage.mcpPicker();
    await expect(advanced).toHaveAttribute("aria-expanded", "false");
    await expect(trigger).toHaveCount(0);
    await expect(dialog.getByRole("combobox").filter({ hasText: "Blank" })).toBeVisible();
    await session.newSessionPromptInput().fill(PROMPT);
    await dialog.locator('input[type="file"]').setInputFiles({
      name: "session-note.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("Keep this attachment"),
    });
    await expect(dialog.getByText("session-note.txt", { exact: true })).toBeVisible();
    await session.newSessionPromptInput().click();
    for (let step = 0; step < 20; step++) {
      await testPage.keyboard.press("Tab");
      if (await advanced.evaluate((el) => el === document.activeElement)) break;
    }
    await expect(advanced).toBeFocused();
    await testPage.keyboard.press("Enter");
    await testPage.keyboard.press("Tab");
    await testPage.keyboard.press("Tab");
    await expect(trigger).toBeFocused();
    await waitForFiniteAnimations(dialog);
    const formHeight = (await dialog.boundingBox())!.height;
    expect((await trigger.boundingBox())!.height).toBeCloseTo(28, 0);
    await testPage.keyboard.press("Enter");
    await expect(picker).toBeVisible();
    await expect(picker).toHaveAttribute("data-slot", "popover-content");
    await expectContainedPicker(testPage, picker);
    expect((await dialog.boundingBox())!.height).toBeCloseTo(formHeight, 0);
    await expect(dialog.getByTestId("new-session-mcp-options")).toHaveCount(1);
    await expect(picker).toContainText("Inherited task documentation");
    const search = picker.getByRole("textbox", { name: "Search MCP servers" });
    await expect(search).toBeFocused();
    await search.fill("Session documentation 1");
    await search.press("Enter");
    await testPage.keyboard.press("Tab");
    const choice = picker.getByTestId(`mcp-choice-${definitions[1].id}`).getByRole("checkbox");
    await expect(choice).toBeFocused();
    await testPage.keyboard.press("Space");
    await expect(choice).toBeChecked();
    await testPage.keyboard.press("Escape");
    await expect(picker).not.toBeVisible();
    await expect(dialog).toBeVisible();
    await expect(trigger).toBeFocused();
    await expect(trigger).toContainText("1 selected");
    await advanced.click();
    await advanced.click();
    await trigger.click();
    await expect(choice).toBeChecked();
    await expectContainedPicker(testPage, picker);
    await testPage.screenshot({ path: testInfo.outputPath("desktop-picker.png") });
    await testPage.keyboard.press("Escape");
    await expect(dialog.getByText("session-note.txt", { exact: true })).toBeVisible();
    await expect(session.newSessionPromptInput()).toHaveValue(PROMPT);
    await dwell(
      testPage,
      200,
      "negative-assertion",
      "Disclosure, search Enter and checkbox actions must not create sessions",
    );
    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);
    await testPage.screenshot({ path: testInfo.outputPath("desktop-advanced.png") });
    await session.newSessionStartButton().click();
    const created = await expectCreatedSelection(
      apiClient,
      seedData,
      task.id,
      task.session_id!,
      definitions[1].id,
    );
    await expect(dialog).not.toBeVisible();
    const messages = (await apiClient.listSessionMessages(created.id)).messages;
    expect(
      messages.find((message) => message.author_type === "user")?.metadata?.attachments,
    ).toEqual(expect.arrayContaining([expect.objectContaining({ name: "session-note.txt" })]));
    await testPage.reload();
    expect(
      (await apiClient.getMCPSelections("task_session", created.id, seedData.workspaceId))
        .definition_ids,
    ).toEqual([definitions[1].id]);
  });

  test("cancel and reopen resets choices and disclosure; loading and empty stay in the picker", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session, definitions } = await seedAdvancedSession(testPage, apiClient, seedData);
    await session.openNewSessionDialog();
    const advanced = session.newSessionDialogPage.advancedTrigger();
    const trigger = session.newSessionDialogPage.mcpTrigger();
    const picker = session.newSessionDialogPage.mcpPicker();
    await advanced.click();
    await trigger.click();
    await picker.getByTestId(`mcp-choice-${definitions[1].id}`).click();
    await testPage.keyboard.press("Escape");
    await session
      .sessionLaunchDialog()
      .getByRole("button", { name: "Cancel", exact: true })
      .click();
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const url = `**/api/v1/workspaces/${seedData.workspaceId}/mcp-servers`;
    await testPage.route(url, async (route) => {
      await held;
      await route.fulfill({ json: { servers: [] } });
    });
    try {
      await session.openNewSessionDialog();
      await expect(advanced).toHaveAttribute("aria-expanded", "false");
      await expect(session.newSessionPromptInput()).toHaveValue("");
      await advanced.click();
      await expect(trigger).toContainText("0 selected");
      await trigger.click();
      await expect(picker).toContainText("Loading MCP servers...");
      release();
      await expect(picker).toContainText("No MCP servers match this search.");
    } finally {
      release();
      await testPage.unroute(url);
    }
  });

  test("uses phone sizing and a sheet at a narrow fine-pointer width", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { session } = await seedAdvancedSession(testPage, apiClient, seedData);
    await testPage.setViewportSize({ width: 767, height: 900 });
    await testPage.getByTestId("mobile-sessions-pill").click();
    await testPage.getByTestId("mobile-launch-session").click();
    const controls = session.newSessionDialogPage;
    await controls.advancedTrigger().click();
    expect((await controls.mcpTrigger().boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await controls.mcpTrigger().click();
    await expect(controls.mcpPicker()).toHaveAttribute("data-slot", "drawer-content");
    await expectContainedPicker(testPage, controls.mcpPicker());
  });

  test.describe("coarse tablet", () => {
    test.use({ hasTouch: true, viewport: { width: 900, height: 1000 } });
    test("keeps selections in a touch sheet", async ({
      testPage,
      apiClient,
      seedData,
    }, testInfo) => {
      const { session, definitions } = await seedAdvancedSession(testPage, apiClient, seedData);
      await testPage.keyboard.press("Control+k");
      const commands = testPage.getByRole("dialog");
      await commands.getByRole("combobox").fill(">New Agent");
      await commands.getByRole("option").filter({ hasText: "New Agent" }).tap();
      await checkTouchPicker(testPage, session, definitions[1].id);
      await session.newSessionDialogPage.mcpTrigger().tap();
      await expectContainedPicker(testPage, session.newSessionDialogPage.mcpPicker());
      await testPage.screenshot({ path: testInfo.outputPath("coarse-tablet-picker.png") });
    });
  });
});
