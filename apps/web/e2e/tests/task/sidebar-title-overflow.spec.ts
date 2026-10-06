import { test, expect } from "../../fixtures/test-base";
import type { Locator } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";

const LONG_TITLE = "Investigate sidebar title overflow without losing the ending";

function taskRow(sidebar: Locator, taskId: string) {
  return sidebar.locator(`[data-task-row-id="${taskId}"]`);
}

function titleElement(row: Locator) {
  return row.locator("span.overflow-hidden").first();
}

async function maskImage(title: Locator) {
  return title.evaluate((element) => {
    const style = getComputedStyle(element);
    return style.maskImage || style.webkitMaskImage;
  });
}

test("sidebar title fades overflow and preserves hover disclosure across row states", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await testPage.addInitScript(() => localStorage.setItem("theme", "light"));
  const stepOptions = {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  };
  const activeTask = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, stepOptions);
  const shortTask = await apiClient.createTask(
    seedData.workspaceId,
    "Short sidebar title",
    stepOptions,
  );
  const parentTask = await apiClient.createTask(
    seedData.workspaceId,
    "Nested title parent",
    stepOptions,
  );
  const nestedTask = await apiClient.createTask(seedData.workspaceId, LONG_TITLE, {
    ...stepOptions,
    parent_id: parentTask.id,
  });

  await testPage.setViewportSize({ width: 1280, height: 900 });
  await testPage.goto(`/t/${activeTask.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.sidebar).toBeVisible();

  const activeRow = taskRow(session.sidebar, activeTask.id);
  const shortRow = taskRow(session.sidebar, shortTask.id);
  const nestedRow = taskRow(session.sidebar, nestedTask.id);
  const activeTitle = titleElement(activeRow);
  const shortTitle = titleElement(shortRow);
  const nestedTitle = titleElement(nestedRow);

  await expect(activeRow).toHaveAttribute("data-active", "true");
  await expect(nestedRow).toHaveAttribute("data-active", "false");
  await expect(activeTitle).toHaveAttribute("data-truncated", "true");
  await expect(nestedTitle).toHaveAttribute("data-truncated", "true");
  await expect(shortTitle).toHaveAttribute("data-truncated", "false");
  await expect(activeTitle).toHaveText(LONG_TITLE);
  await expect(nestedTitle).toHaveText(LONG_TITLE);
  await expect.poll(() => maskImage(activeTitle)).toContain("linear-gradient");
  await expect.poll(() => maskImage(nestedTitle)).toContain("linear-gradient");
  await expect.poll(() => maskImage(shortTitle)).toBe("none");

  const activeGeometry = await activeTitle.evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
    box: element.getBoundingClientRect().toJSON(),
    rowMask: getComputedStyle(element.closest('[data-testid="sidebar-task-item"]')!).maskImage,
  }));
  expect(activeGeometry.scrollWidth).toBeGreaterThan(activeGeometry.clientWidth);
  expect(activeGeometry.rowMask).toBe("none");

  const themeCases = ["light", "dark"] as const;

  for (const theme of themeCases) {
    if (theme === "dark") {
      const toggle = testPage.getByRole("button", { name: "Switch to Dark Mode", exact: true });
      await expect(toggle).toBeVisible();
      await toggle.evaluate((element) => (element as HTMLButtonElement).click());
      await expect(testPage.locator("html")).toHaveClass(/(^|\s)dark(\s|$)/);
    } else {
      await expect(testPage.locator("html")).not.toHaveClass(/(^|\s)dark(\s|$)/);
    }

    await testPage.mouse.move(0, 0);
    await expect.poll(() => maskImage(activeTitle)).toContain("linear-gradient");
    await expect.poll(() => maskImage(nestedTitle)).toContain("linear-gradient");
    await prCapture.screenshot(`sidebar-title-${theme}-rows`, {
      caption: `${theme} theme with selected and default overflowing task titles`,
    });

    const nestedBoxBeforeHover = await nestedTitle.boundingBox();
    expect(nestedBoxBeforeHover).not.toBeNull();
    await nestedTitle.hover();
    await expect.poll(() => maskImage(nestedTitle)).toBe("none");
    const nestedText = nestedTitle.locator(":scope > span");
    await expect
      .poll(() => nestedText.evaluate((element) => (element as HTMLElement).style.transform))
      .toMatch(/^translateX\(-/);
    await expect
      .poll(
        () =>
          nestedText.evaluate((element) => {
            const titleBounds = element.parentElement!.getBoundingClientRect();
            return element.getBoundingClientRect().right <= titleBounds.right + 1;
          }),
        { timeout: 12_000 },
      )
      .toBe(true);
    const nestedBoxWhileHovered = await nestedTitle.boundingBox();
    expect(nestedBoxWhileHovered).not.toBeNull();
    expect(nestedBoxWhileHovered!.width).toBeCloseTo(nestedBoxBeforeHover!.width, 1);
    await prCapture.screenshot(`sidebar-title-${theme}-hover`, {
      caption: `${theme} theme with the task title ending revealed on hover`,
    });

    await testPage.mouse.move(0, 0);
    await expect.poll(() => maskImage(nestedTitle)).toContain("linear-gradient");
    await expect
      .poll(() => nestedText.evaluate((element) => (element as HTMLElement).style.transform))
      .toBe("");
  }

  await testPage.setViewportSize({ width: 767, height: 900 });
  await expect(session.sidebar).toBeHidden();
  const mobilePickerTrigger = testPage.getByTestId("mobile-task-picker-trigger");
  await expect(mobilePickerTrigger).toBeVisible();
  await mobilePickerTrigger.click();
  const picker = testPage.getByRole("dialog", { name: "Tasks" });
  const pickerTitle = titleElement(taskRow(picker, activeTask.id));
  await expect(pickerTitle).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(picker).toBeHidden();

  await testPage.setViewportSize({ width: 768, height: 900 });
  await expect(session.sidebar).toBeVisible();
  await expect(activeTitle).toHaveAttribute("data-truncated", "true");
  await expect.poll(() => maskImage(activeTitle)).toContain("linear-gradient");
});
