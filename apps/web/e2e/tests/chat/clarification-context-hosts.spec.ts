import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { openQuickChatWithAgent, sendQuickChatMessage } from "./quick-chat-helpers";

async function readHostGeometry(container: Locator) {
  return container.evaluate((element) => ({
    hostMax: Number((element as HTMLElement).dataset.hostMaxHeight),
    maxHeight: Number.parseFloat((element as HTMLElement).style.maxHeight),
    height: element.getBoundingClientRect().height,
    viewportHeight: window.innerHeight,
  }));
}

test("Quick Chat expands formatted context to its measured allocation and restores the initial cap", async ({
  testPage,
}, testInfo) => {
  await testPage.setViewportSize({ width: 1280, height: 1200 });
  const dialog = await openQuickChatWithAgent(testPage);
  await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-markdown");

  const container = dialog.getByTestId("clarification-overlay-container");
  await expect(container).toBeVisible({ timeout: 30_000 });
  await expect(dialog.getByTestId("clarification-context").locator("table")).toBeVisible();
  const initial = await readHostGeometry(container);
  expect(initial.hostMax).toBeGreaterThan(initial.maxHeight);
  expect(initial.maxHeight).toBe(Math.round(initial.viewportHeight * 0.35));
  await expect(dialog.getByTestId("chat-input-editor")).toBeVisible();

  const toggle = dialog.getByTestId("clarification-panel-size-toggle");
  await expect(toggle).toHaveCount(1);
  await expect(toggle).toHaveAttribute("aria-label", "Expand");
  await expect(toggle).toBeEnabled();
  await toggle.click();
  await expect(container).toHaveCSS("height", `${initial.hostMax}px`);
  await expect(container).toHaveCSS("max-height", `${initial.hostMax}px`);
  await expect(toggle).toHaveAttribute("aria-label", "Restore");
  await expect(dialog.getByTestId("chat-input-editor")).toBeVisible();
  await testInfo.attach("quick-chat-clarification-expanded.png", {
    body: await testPage.screenshot(),
    contentType: "image/png",
  });

  await toggle.click();
  await expect
    .poll(() => container.evaluate((element) => (element as HTMLElement).style.height))
    .toBe("");
  await expect(container).toHaveCSS("max-height", `${initial.maxHeight}px`);
  await expect(toggle).toHaveAttribute("aria-label", "Expand");

  await testPage.setViewportSize({ width: 767, height: 1200 });
  const phoneBox = await toggle.boundingBox();
  expect(phoneBox).not.toBeNull();
  expect(phoneBox!.height).toBeGreaterThanOrEqual(44);
  expect(phoneBox!.width).toBeGreaterThanOrEqual(44);
  await testPage.setViewportSize({ width: 768, height: 1200 });
  const desktopBox = await toggle.boundingBox();
  expect(desktopBox).not.toBeNull();
  expect(desktopBox!.height).toBeLessThan(44);
  expect(desktopBox!.width).toBeLessThan(44);
});

test("keeps Quick Chat's rendered panel within a host below 120px", async ({ testPage }) => {
  await testPage.setViewportSize({ width: 1280, height: 1200 });
  const dialog = await openQuickChatWithAgent(testPage);
  await sendQuickChatMessage(dialog, testPage, "/e2e:clarification-markdown");

  const container = dialog.getByTestId("clarification-overlay-container");
  await expect(container).toBeVisible({ timeout: 30_000 });
  await testPage.setViewportSize({ width: 1280, height: 400 });
  await expect
    .poll(async () => {
      const value = await container.getAttribute("data-host-max-height");
      return value === null || value.trim() === "" ? Infinity : Number(value);
    })
    .toBeLessThan(120);

  const constrained = await readHostGeometry(container);
  expect(constrained.hostMax).toBeGreaterThan(0);
  expect(constrained.height).toBeLessThanOrEqual(constrained.hostMax + 1);
});

test("Needs You bounds a long clarification to the inbox viewport and keeps its answer reachable", async ({
  testPage,
  apiClient,
  seedData,
}, testInfo) => {
  await testPage.setViewportSize({ width: 393, height: 900 });
  const title = "Needs You Clarification Host Bound";
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:clarification-markdown",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("expected a clarification session for the Needs You row");

  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "the Needs You host test needs a parked clarification",
    timeout: 60_000,
  });
  await testPage.goto("/needs-you-inbox");

  const row = testPage.getByTestId("needs-you-inbox-row").filter({ hasText: title });
  await expect(row).toBeVisible({ timeout: 30_000 });
  await row.scrollIntoViewIfNeeded();
  await row.getByTestId("needs-you-inbox-row-toggle").click();
  const container = row.getByTestId("clarification-overlay-container");
  await expect(container).toBeVisible();

  const initial = await readHostGeometry(container);
  expect(initial.hostMax).toBeGreaterThan(0);
  expect(initial.maxHeight).toBe(
    Math.min(Math.round(initial.viewportHeight * 0.5), initial.hostMax),
  );
  const toggle = row.getByTestId("clarification-panel-size-toggle");
  await expect(toggle).toHaveCount(1);
  await expect(toggle).toHaveAttribute("aria-label", "Expand");
  await expect(toggle).toBeEnabled();
  await toggle.click();
  await expect(container).toHaveCSS("height", `${initial.hostMax}px`);
  await expect(container).toHaveCSS("max-height", `${initial.hostMax}px`);
  await expect(toggle).toHaveAttribute("aria-label", "Restore");

  const scrollRegion = row.getByTestId("clarification-scroll-region");
  const answer = row.getByTestId("clarification-option").first();
  const regionBox = await scrollRegion.boundingBox();
  if (!regionBox) throw new Error("Needs You clarification scroll region is not measurable");
  await testPage.mouse.move(regionBox.x + regionBox.width / 2, regionBox.y + regionBox.height / 2);
  for (let step = 0; step < 8; step += 1) {
    const [currentRegion, answerBox] = await Promise.all([
      scrollRegion.boundingBox(),
      answer.boundingBox(),
    ]);
    if (
      currentRegion &&
      answerBox &&
      answerBox.y >= currentRegion.y &&
      answerBox.y + answerBox.height <= currentRegion.y + currentRegion.height
    ) {
      break;
    }
    await testPage.mouse.wheel(0, 360);
  }
  const [finalRegion, finalAnswer] = await Promise.all([
    scrollRegion.boundingBox(),
    answer.boundingBox(),
  ]);
  expect(finalRegion).not.toBeNull();
  expect(finalAnswer).not.toBeNull();
  expect(finalAnswer!.y).toBeGreaterThanOrEqual(finalRegion!.y - 1);
  expect(finalAnswer!.y + finalAnswer!.height).toBeLessThanOrEqual(
    finalRegion!.y + finalRegion!.height + 1,
  );
  await testInfo.attach("needs-you-clarification-expanded.png", {
    body: await testPage.screenshot(),
    contentType: "image/png",
  });
  await answer.click();
  await row.getByTestId("clarification-custom-submit").click();
  await expect(row).toHaveCount(0, { timeout: 15_000 });
});
