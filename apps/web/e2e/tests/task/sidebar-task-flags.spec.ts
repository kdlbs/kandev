import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";

test("adds, reads, edits, and removes a flag on another sidebar task", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const viewport = { width: 1280, height: 480 };
  await testPage.setViewportSize(viewport);
  const options = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const source = await apiClient.seedTask(seedData.workspaceId, "Current task", options);
  const target = await apiClient.seedTask(seedData.workspaceId, "Review release notes", options);
  await testPage.goto(`/t/${source.task_id}`);
  await new SessionPage(testPage).waitForLoad();
  const sidebar = new SidebarTasksPage(testPage);
  await sidebar.rightClick(target.task_id);
  await sidebar.menuItem("Add flag").click();

  const editor = testPage.getByRole("dialog").filter({ has: testPage.getByRole("textbox") });
  const save = editor.getByRole("button", { name: "Save", exact: true });
  await expect(save).toBeDisabled();
  const note = editor.getByRole("textbox");
  await expect(note).toHaveAttribute("maxlength", "1024");
  await waitForFiniteAnimations(editor);
  await note.fill("Check links before release");
  expect((await editor.boundingBox())!.height).toBeLessThan(viewport.height / 2);

  await note.fill(Array.from({ length: 96 }, (_, index) => `Check ${index}`).join("\n"));
  const noteScroll = await note.evaluate((element) => ({
    height: element.clientHeight,
    content: element.scrollHeight,
  }));
  expect(noteScroll.content).toBeGreaterThan(noteScroll.height);
  const editorBounds = (await editor.boundingBox())!;
  expect(editorBounds.y).toBeGreaterThanOrEqual(0);
  expect(editorBounds.y + editorBounds.height).toBeLessThanOrEqual(viewport.height);
  await expect(save).toBeInViewport({ ratio: 1 });

  await note.fill("Check links before release");
  await save.click();
  const flag = sidebar.row(target.task_id).getByRole("button", { name: /flag/i });
  await flag.hover();
  await expect(testPage.getByRole("tooltip")).toHaveText("Check links before release");

  await sidebar.rightClick(target.task_id);
  await sidebar.menuItem("Flag").click();
  await expect(editor.getByRole("textbox")).toHaveValue("Check links before release");
  await editor.getByRole("textbox").fill("Check the final link");
  await save.click();
  await expect(editor).toBeHidden();
  await testPage.reload();
  await flag.focus();
  await expect(testPage.getByRole("tooltip")).toHaveText("Check the final link");
  await sidebar.rightClick(target.task_id);
  await sidebar.menuItem("Flag").click();
  await editor.getByRole("button", { name: "Remove flag" }).click();
  await expect(flag).toHaveCount(0);
  await sidebar.rightClick(target.task_id);
  await expect(sidebar.menuItem("Add flag")).toBeVisible();
  expect(new URL(testPage.url()).pathname).toBe(`/t/${source.task_id}`);
});
