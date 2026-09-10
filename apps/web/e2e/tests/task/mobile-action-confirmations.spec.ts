import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { MobileKanbanPage } from "../../pages/mobile-kanban-page";

const presentations = [
  {
    locale: "pt-pt",
    width: 320,
    height: 640,
    theme: "light",
    more: "Mais opções",
    archive: "Arquivar",
    cancel: "Cancelar",
  },
  {
    locale: "en",
    width: 767,
    height: 900,
    theme: "dark",
    more: "More options",
    archive: "Archive",
    cancel: "Cancel",
  },
  {
    locale: "pseudo",
    width: 667,
    height: 375,
    theme: "light",
    more: "Ḿōŕē ōƥţĩōńś",
    archive: "Àŕćĥĩvē",
    cancel: "Ćàńćēĺ",
  },
] as const;

for (const presentation of presentations) {
  test(`standalone archive fits ${presentation.width}px ${presentation.locale} ${presentation.theme}`, async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.setViewportSize(presentation);
    await testPage.addInitScript(({ theme }) => {
      localStorage.setItem("theme", theme);
    }, presentation);
    const task = await apiClient.createTask(
      seedData.workspaceId,
      `Long mobile title ${"X".repeat(40)}`,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    const board = new MobileKanbanPage(testPage);
    await board.goto();
    await testPage.evaluate((locale) => {
      document.cookie = `kandev_locale=${locale}; path=/; SameSite=Lax`;
    }, presentation.locale);
    await testPage.reload();
    await expect(testPage.locator("html")).toHaveAttribute("lang", presentation.locale);
    const card = board.taskCard(task.id);
    await card.getByRole("button", { name: presentation.more, exact: true }).tap();
    await testPage.getByRole("menuitem", { name: presentation.archive, exact: true }).tap();
    const dialog = testPage
      .getByRole("dialog")
      .filter({ has: testPage.getByTestId("mobile-action-confirmation") });
    await expect(dialog).toContainText(task.title);
    await waitForFiniteAnimations(dialog);
    await expect(testPage.getByText("Kandev update available", { exact: true })).toBeHidden();
    await assertNoDocumentHorizontalOverflow(testPage);
    const confirm = dialog.getByRole("button", { name: presentation.archive, exact: true });
    const cancel = dialog.getByRole("button", { name: presentation.cancel, exact: true });
    const boxes = [];
    for (const button of [confirm, cancel]) {
      const box = (await button.boundingBox())!;
      boxes.push(box);
      expect(box.height).toBeGreaterThanOrEqual(48);
      expect(box.y).toBeGreaterThanOrEqual(0);
      expect(box.y + box.height).toBeLessThanOrEqual(presentation.height);
      expect(
        await button.evaluate((element) => {
          const bounds = element.getBoundingClientRect();
          return element.contains(
            document.elementFromPoint(bounds.x + bounds.width / 2, bounds.y + bounds.height / 2),
          );
        }),
      ).toBe(true);
    }
    expect(boxes[0].y + boxes[0].height).toBeLessThanOrEqual(boxes[1].y);
    await expect(cancel).toBeFocused();
    await prCapture.screenshot(
      `standalone-${presentation.width}-${presentation.locale}-${presentation.theme}`,
    );
    const dialogId = await dialog.getAttribute("id");
    await testPage.setViewportSize({ width: 393, height: 700 });
    await expect(dialog).toHaveAttribute("id", dialogId!);
    await expect(cancel).toBeVisible();
    await testPage.setViewportSize({ width: 768, height: 800 });
    await expect(testPage.getByTestId("mobile-action-confirmation")).toHaveCount(0);
    const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${task.id}`);
    expect((await response.json()).archived_at).toBeFalsy();
  });
}

test("archive is a step in the Tasks drawer and Cancel restores the scrolled list", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const options = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  };
  const active = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Active mobile context",
    seedData.agentProfileId,
    { ...options, description: 'e2e:message("Ready")' },
  );
  const target = await apiClient.createTask(
    seedData.workspaceId,
    "Archive target with a long title that wraps on a phone",
    options,
  );
  for (let index = 0; index < 18; index++) {
    await apiClient.createTask(seedData.workspaceId, `Mobile neighboring task ${index}`, options);
  }
  await testPage.goto(`/t/${active.id}`);
  await new SessionPage(testPage).waitForLoad();
  await testPage.getByTestId("mobile-session-menu").tap();
  const sheet = testPage
    .locator('[data-slot="drawer-content"]')
    .filter({ has: testPage.getByRole("heading", { name: "Tasks", exact: true }) });
  await expect(sheet).toBeVisible();
  const sheetId = await sheet.getAttribute("id");
  const row = sheet.getByTestId("sidebar-task-item").filter({ hasText: target.title });
  await row.scrollIntoViewIfNeeded();
  const list = sheet.getByTestId("mobile-task-switcher-list");
  const scrollTop = await list.evaluate((element) => element.scrollTop);
  const originalHeight = (await sheet.boundingBox())!.height;
  await row.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByRole("menuitem", { name: "Archive", exact: true }).tap();
  const confirmation = testPage.getByRole("dialog", { name: "Archive task?", exact: true });
  await expect(confirmation).toBeVisible();
  await expect(confirmation).toHaveAttribute("id", sheetId!);
  await expect(testPage.locator('[data-slot="drawer-content"]')).toHaveCount(1);
  await expect(confirmation).toContainText(target.title);
  await expect(list).toBeHidden();
  await expect(testPage.getByTestId("task-archive-inline-confirmation")).toHaveCount(0);
  await waitForFiniteAnimations(confirmation);
  expect((await confirmation.boundingBox())!.height).toBeCloseTo(originalHeight, 0);
  await assertNoDocumentHorizontalOverflow(testPage);
  const cancel = confirmation.getByRole("button", { name: "Cancel", exact: true });
  await expect(cancel).toBeFocused();
  await prCapture.screenshot("hosted-archive", {
    caption: "Archive becomes a focused step in the same Tasks sheet.",
  });
  await cancel.tap();
  await expect(testPage.getByRole("dialog", { name: "Tasks", exact: true })).toBeVisible();
  await expect(list).toBeVisible();
  expect(await list.evaluate((element) => element.scrollTop)).toBe(scrollTop);
  await expect(row).toBeVisible();
  await expect(testPage).toHaveURL(new RegExp(`/t/${active.id}`));
});
