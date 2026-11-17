import { dwell } from "../../helpers/causal-waits";
import { chooseSidebarColor } from "../../helpers/bulk-task-colors";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { test, expect } from "../../fixtures/test-base";
import {
  openAgentReferences,
  REFERENCE_TITLE,
  REFERENCE_DESCRIPTION,
  REFERENCE_PARENT,
} from "./agent-task-reference-helpers";

// @covers AC-UI-AGENT-TASK-REFERENCE-001.1
test("references follow personal task colors and restore the accent when cleared", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { target, sibling, message, session } = await openAgentReferences(
    testPage,
    apiClient,
    seedData,
  );
  const link = message.locator(`a[href="/t/${target.id}"]`);
  const other = message.locator(`a[href="/t/${sibling.id}"]`);
  const color = () => link.evaluate((element) => getComputedStyle(element).color);
  const accent = await color();
  const sidebar = new SidebarTasksPage(testPage);
  await sidebar.row(target.id).click({ button: "right" });
  await chooseSidebarColor(testPage, "Red");
  await expect.poll(color).not.toBe(accent);
  const red = await color();
  await expect(other).toHaveCSS("color", accent);
  await testPage.reload();
  await session.showSessionContext();
  await expect.poll(color).toBe(red);
  await apiClient.saveUserSettings({
    sidebar_task_color_patch: { colors: { [target.id]: "green" } },
  });
  await expect.poll(color).not.toBe(red);
  await expect.poll(color).not.toBe(accent);
  await sidebar.row(target.id).click({ button: "right" });
  await chooseSidebarColor(testPage, "None");
  await expect.poll(color).toBe(accent);
});

// @covers AC-UI-AGENT-TASK-REFERENCE-001.1, AC-UI-AGENT-TASK-REFERENCE-001.2
// @covers AC-UI-AGENT-TASK-REFERENCE-001.3, AC-UI-AGENT-TASK-REFERENCE-001.5
// @covers AC-UI-AGENT-TASK-REFERENCE-001.6, AC-UI-AGENT-TASK-REFERENCE-001.7
test("agent references preserve identity and navigate without reloading", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const { target, sibling, source, message, session } = await openAgentReferences(
    testPage,
    apiClient,
    seedData,
  );
  const link = message.locator(`a[href="/t/${target.id}"]`);
  await expect(link).toHaveText(REFERENCE_TITLE);
  await expect(link.locator("svg")).toHaveCount(1);
  await expect(message.locator(`a[href="/t/${sibling.id}"]`)).toHaveText(REFERENCE_TITLE);
  await expect(message.getByRole("link", { name: "example" })).toHaveCount(0);
  const metadataReads: string[] = [];
  const observeMetadata = (request: import("@playwright/test").Request) => {
    if (request.url().includes(`/api/v1/tasks/${target.id}`)) metadataReads.push(request.url());
  };
  testPage.on("request", observeMetadata);
  await testPage.mouse.move(0, 0);
  await link.hover();
  await expect(testPage.getByRole("tooltip")).toContainText(REFERENCE_TITLE);
  await expect(testPage.getByRole("tooltip")).not.toContainText(REFERENCE_DESCRIPTION);
  await expect(testPage.getByRole("tooltip")).toContainText("E2E Workflow");
  await expect(testPage.getByRole("tooltip")).toContainText("2 sessions");
  await expect(testPage.getByRole("tooltip")).toContainText(REFERENCE_PARENT);
  await expect(testPage.getByRole("tooltip").locator("time")).toHaveAttribute(
    "datetime",
    /^\d{4}-\d{2}-\d{2}T/,
  );
  await expect(testPage.getByRole("tooltip").locator('[data-slot="badge"]').first()).toContainText(
    "Created",
  );
  await dwell(
    testPage,
    250,
    "negative-assertion",
    "Task reference hover must not request task metadata",
  );
  expect(metadataReads).toEqual([]);
  testPage.off("request", observeMetadata);
  await testPage.mouse.move(0, 0);
  await testPage.locator('[data-testid^="session-tab-"]:visible').first().click();
  for (let step = 0; step < 100; step++) {
    await testPage.keyboard.press("Tab");
    if (await link.evaluate((el) => el === document.activeElement)) break;
  }
  await expect(link).toBeFocused();
  const tooltip = testPage.getByRole("tooltip").filter({ hasText: REFERENCE_TITLE });
  await expect(tooltip).toContainText(REFERENCE_TITLE);
  await link.press("Escape");
  await expect(tooltip).toHaveCount(0);
  await expect(link).toBeFocused();
  for (const theme of ["light", "dark"]) {
    await testPage.evaluate((value) => {
      document.documentElement.classList.remove("light", "dark");
      document.documentElement.classList.add(value);
    }, theme);
    await apiClient.saveUserSettings({
      sidebar_task_color_patch: { colors: { [target.id]: "green", [sibling.id]: "orange" } },
    });
    await expect(link).toHaveClass(/text-green-800/);
    await link.hover();
    await expect(tooltip).toBeVisible();
    await waitForFiniteAnimations(testPage.locator('[data-slot="tooltip-content"]'));
    await prCapture.screenshot(`agent-task-reference-${theme}`, {
      caption: `Task colors and metadata preview (${theme})`,
    });
  }
  await apiClient.updateTaskTitle(target.id, "Renamed reconnect task");
  await testPage.reload();
  await session.showSessionContext();
  await expect(link).toHaveText(REFERENCE_TITLE);
  await testPage.evaluate(() => {
    (window as unknown as Record<string, unknown>).referenceNavigationMarker = true;
  });
  const documents: string[] = [];
  testPage.on("request", (request) => {
    if (request.isNavigationRequest() && request.frame() === testPage.mainFrame())
      documents.push(request.url());
  });
  await link.press("Enter");
  await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}(?:\\?|$)`));
  expect(
    await testPage.evaluate(
      () => (window as unknown as Record<string, unknown>).referenceNavigationMarker,
    ),
  ).toBe(true);
  expect(documents).toEqual([]);
  await testPage.goBack();
  await expect(testPage).toHaveURL(new RegExp(`/t/${source.id}(?:\\?|$)`));
  await testPage.setViewportSize({ width: 700, height: 900 });
  await expect(link).toHaveCSS("min-height", "24px");
  await message.locator(`a[href="/t/${sibling.id}"]`).click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${sibling.id}(?:\\?|$)`));
  await testPage.goBack();
  await expect(testPage).toHaveURL(new RegExp(`/t/${source.id}(?:\\?|$)`));
  await apiClient.deleteTask(target.id);
  await link.click();
  await expect(testPage.getByTestId("task-load-error-state")).toContainText("Task unavailable");
});

// @covers AC-UI-AGENT-TASK-REFERENCE-001.1, AC-UI-AGENT-TASK-REFERENCE-001.6
test("agent plan references work in the expanded plan dialog", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { target, source, session } = await openAgentReferences(testPage, apiClient, seedData);
  await apiClient.seedSessionMessage(source.session_id!, {
    type: "agent_plan",
    content: `# Follow-up plan\nOpen [Target task](/t/${target.id}).`,
    authorType: "agent",
  });
  await testPage.reload();
  await session.showSessionContext();
  const planLink = session.activeChat().getByRole("link", { name: "Target task", exact: true });
  await expect(planLink.locator("svg")).toHaveCount(1);
  await session
    .activeChat()
    .getByRole("button", { name: "Open plan details", exact: true })
    .click();
  const dialog = testPage.getByRole("dialog");
  const link = dialog.getByRole("link", { name: "Target task", exact: true });
  await expect(link.locator("svg")).toHaveCount(1);
  await link.click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${target.id}(?:\\?|$)`));
});
