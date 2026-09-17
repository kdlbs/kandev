import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { openQuickChatWithAgent } from "./quick-chat-helpers";
import {
  assertPickerContained,
  seedLongModelList,
  wheelModelList,
} from "./quick-chat-model-scroll-helpers";

test.describe("Quick Chat model scrolling", () => {
  // @covers AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.7 AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.8
  test("scrolls models inside the modal and selects a revealed model", async ({ testPage }) => {
    await testPage.setViewportSize({ width: 1440, height: 800 });
    const dialog = await openQuickChatWithAgent(testPage);
    const trigger = dialog.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast");
    await trigger.click();
    await seedLongModelList(testPage);
    const list = testPage.getByRole("listbox");
    await expect(list.getByRole("option")).toHaveCount(27);
    const composerBefore = await dialog.locator(".tiptap.ProseMirror").boundingBox();
    const backgroundBefore = await testPage.evaluate(() => window.scrollY);
    const smart = list.getByRole("option", { name: /Mock Smart/ });
    await expect(smart).not.toBeInViewport();
    await wheelModelList(testPage, list);
    await expect(smart).toBeInViewport();
    await assertPickerContained(testPage);
    await smart.click();
    await expect(trigger).toContainText("Mock Smart");
    expect(await testPage.evaluate(() => window.scrollY)).toBe(backgroundBefore);
    expect((await dialog.locator(".tiptap.ProseMirror").boundingBox())!.y).toBe(composerBefore!.y);
  });

  test("preserves search, Escape focus return, and keyboard selection", async ({ testPage }) => {
    const dialog = await openQuickChatWithAgent(testPage);
    const trigger = dialog.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast");
    await trigger.click();
    await seedLongModelList(testPage);
    const search = testPage.getByPlaceholder("Filter models...");
    await search.fill("Mock Smart");
    await expect(search).toBeFocused();
    await expect(testPage.getByRole("option", { name: "Mock Smart", exact: true })).toBeVisible();
    await expect(testPage.getByRole("option", { name: "Scroll model 1", exact: true })).toHaveCount(
      0,
    );
    await search.press("Escape");
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await expect(dialog).toBeVisible();
    await expect(trigger).toBeFocused();

    await trigger.click();
    await search.fill("Mock Smart");
    await search.press("Home");
    await search.press("Enter");
    await expect(trigger).toContainText("Mock Smart");
  });

  test("keeps non-modal task model scrolling and selection working", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model scroll fallback",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast");
    await trigger.click();
    await seedLongModelList(testPage, task.session_id!);
    const list = testPage.getByRole("listbox");
    const smart = list.getByRole("option", { name: /Mock Smart/ });
    await expect(smart).not.toBeInViewport();
    await wheelModelList(testPage, list);
    await expect(smart).toBeInViewport();
    await smart.click();
    await expect(trigger).toContainText("Mock Smart");
  });
});
