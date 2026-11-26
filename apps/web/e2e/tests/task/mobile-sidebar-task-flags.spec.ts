import { expect, test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import { writeSidebarFlag } from "./sidebar-task-flags-helpers";

test("adds a flag from phone task actions and opens its note by touch", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const options = { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId };
  const source = await apiClient.seedTask(seedData.workspaceId, "Current task", options);
  const title = "Review release notes";
  const target = await apiClient.seedTask(seedData.workspaceId, title, options);
  await testPage.goto(`/t/${source.task_id}`);
  await new SessionPage(testPage).waitForLoad();
  const picker = testPage.getByTestId("mobile-task-picker-trigger");
  await picker.tap();
  const tasks = testPage.getByRole("dialog", { name: "Tasks", exact: true });
  const row = tasks.getByTestId("sidebar-task-item").filter({ hasText: title });
  await row.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByRole("menuitem", { name: "Add flag" }).tap();
  const editor = testPage.getByRole("dialog").filter({ has: testPage.getByRole("textbox") });
  await expect(editor).toHaveAttribute("data-slot", "drawer-content");
  const note = editor.getByRole("textbox");
  const save = editor.getByRole("button", { name: "Save", exact: true });
  const initialNote = "Review the final link";
  const updatedNote = "Check the release date";
  await expect(note).toHaveAttribute("maxlength", "1024");
  await note.fill(initialNote);
  await assertNoDocumentHorizontalOverflow(testPage, "phone flag editor");
  await writeSidebarFlag(testPage, target.task_id, initialNote, () => save.tap());
  await expect(editor).toBeHidden();
  await picker.tap();
  const flag = row.getByRole("button", { name: /flag/i });
  await expect(flag).toBeVisible();
  const bounds = await flag.boundingBox();
  expect(bounds?.width).toBeGreaterThanOrEqual(44);
  expect(bounds?.height).toBeGreaterThanOrEqual(44);
  await flag.tap();
  await expect(note).toHaveValue(initialNote);
  await note.fill(updatedNote);
  await writeSidebarFlag(testPage, target.task_id, updatedNote, () => save.tap());
  await expect(editor).toBeHidden();
  await testPage.reload();
  await picker.tap();
  await row.getByRole("button", { name: "Task actions" }).tap();
  await testPage.getByRole("menuitem", { name: "Flag", exact: true }).tap();
  await expect(note).toHaveValue(updatedNote);
  await writeSidebarFlag(testPage, target.task_id, "", () =>
    editor.getByRole("button", { name: "Remove flag" }).tap(),
  );
  await expect(editor).toBeHidden();
  await picker.tap();
  await expect(row).toBeVisible();
  await expect(flag).toHaveCount(0);
  expect(new URL(testPage.url()).pathname).toBe(`/t/${source.task_id}`);
});
