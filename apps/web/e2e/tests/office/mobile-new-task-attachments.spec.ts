import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  attachCreationFiles,
  chooseProject,
  createAttachmentProject,
  newTaskDialog,
  openNewTaskDialog,
  pasteReadableImage,
} from "./new-task-attachments-helpers";

test("phone Office creation supports attachments, preview, and submission", async ({
  testPage,
  officeSeed,
  officeApi,
}) => {
  const project = await createAttachmentProject(
    officeApi,
    officeSeed.workspaceId,
    "Mobile office attachments",
  );
  const dialog = await openNewTaskDialog(testPage, true);
  await dialog.getByPlaceholder("Task title").fill("Mobile attachment task");
  await chooseProject(testPage, dialog, project.name);

  await attachCreationFiles(dialog);
  await pasteReadableImage(dialog);
  const create = dialog.getByTestId("new-task-create-button");
  await expect(create).toBeEnabled();

  await dialog
    .getByRole("button", { name: /^Image \(/ })
    .first()
    .tap();
  const drawer = testPage.getByRole("dialog", { name: /^Image \(/ });
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: "Open" }).tap();
  await expect(testPage.getByRole("dialog", { name: "Image preview" })).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("dialog", { name: "Image preview" })).toBeHidden();

  const overflow = await testPage.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth,
  );
  expect(overflow).toBe(false);

  const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
  await create.tap();
  const response = await created;
  const body = (await response.json()) as { id?: string };
  expect(body.id).toBeTruthy();
  const requestBody = response.request().postDataJSON() as {
    attachments?: Array<{ attachment_id?: string }>;
  };
  expect(requestBody.attachments?.length).toBe(3);
  expect(requestBody.attachments?.every((attachment) => Boolean(attachment.attachment_id))).toBe(
    true,
  );
  await expect(newTaskDialog(testPage)).toBeHidden();
});
