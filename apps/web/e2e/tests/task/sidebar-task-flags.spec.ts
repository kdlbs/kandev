import { expect, test } from "../../fixtures/test-base";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";
import { SidebarTasksPage } from "../../pages/sidebar-tasks-page";
import { failNextSidebarFlagWrite, writeSidebarFlag } from "./sidebar-task-flags-helpers";

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
  const initialNote = "Check links before release";
  const updatedNote = "Check the final link";
  await expect(save).toBeDisabled();
  const note = editor.getByRole("textbox");
  await expect(note).toHaveAttribute("maxlength", "1024");
  await waitForFiniteAnimations(editor);
  await note.fill(initialNote);
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

  await note.fill(initialNote);
  await writeSidebarFlag(testPage, target.task_id, initialNote, () => save.click());
  await expect(editor).toBeHidden();
  const row = sidebar.row(target.task_id);
  const flag = row.getByRole("button", { name: "Flag", exact: true });
  const tooltip = testPage.getByRole("tooltip");
  const editDialog = testPage.getByRole("dialog", { name: "Edit flag", exact: true });
  const editNote = editDialog.getByRole("textbox", { name: "Flag note" });
  const editSave = editDialog.getByRole("button", { name: "Save", exact: true });

  await flag.hover();
  await expect(tooltip).toHaveText(initialNote);
  await expect(editor).toBeHidden();

  await sidebar.rightClick(target.task_id);
  await sidebar.menuItem("Change flag").hover();
  await sidebar.menuItem("Edit flag").click();
  await expect(editDialog).toHaveAttribute("data-slot", "dialog-content");
  await expect(editNote).toHaveValue(initialNote);
  await expect(editNote).toBeFocused();
  await editNote.fill(updatedNote);
  await failNextSidebarFlagWrite(testPage, target.task_id);
  await editSave.click();
  await expect(editDialog.getByRole("alert")).toContainText("Could not update the flag");
  await expect(editNote).toHaveValue(updatedNote);
  await writeSidebarFlag(testPage, target.task_id, updatedNote, () => editSave.click());
  await expect(editDialog).toBeHidden();

  await testPage.reload();
  await row.focus();
  await testPage.keyboard.press("Tab");
  await expect(flag).toBeFocused();
  await expect(tooltip).toHaveText(updatedNote);
  await flag.press("Enter");
  await expect(editor).toBeHidden();

  await sidebar.rightClick(target.task_id);
  await sidebar.menuItem("Change flag").hover();
  await sidebar.menuItem("Edit flag").click();
  await expect(editNote).toHaveValue(updatedNote);
  await editNote.fill("Discard this edit");
  await editDialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(editDialog).toBeHidden();
  await expect(row).toBeFocused();
  await expect
    .poll(async () => (await apiClient.getTask(target.task_id)).flag_note)
    .toBe(updatedNote);

  await sidebar.rightClick(target.task_id);
  await sidebar.menuItem("Change flag").hover();
  await failNextSidebarFlagWrite(testPage, target.task_id);
  await sidebar.menuItem("Remove flag").click();
  await expect(testPage.getByText("Could not update the flag. Try again.")).toBeVisible();
  await expect(flag).toBeVisible();
  await sidebar.rightClick(target.task_id);
  await sidebar.menuItem("Change flag").hover();
  await writeSidebarFlag(testPage, target.task_id, "", () =>
    sidebar.menuItem("Remove flag").click(),
  );
  await expect(editDialog).toBeHidden();
  await expect(flag).toHaveCount(0);
  await sidebar.rightClick(target.task_id);
  await expect(sidebar.menuItem("Add flag")).toBeVisible();
  expect(new URL(testPage.url()).pathname).toBe(`/t/${source.task_id}`);
});
