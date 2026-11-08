import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import {
  attachCreationFiles,
  chooseProject,
  createAttachmentProject,
  newTaskDialog,
  openNewTaskDialog,
} from "./new-task-attachments-helpers";

test("Office creation uploads, previews, and submits attachments", async ({
  testPage,
  officeSeed,
  officeApi,
}) => {
  const project = await createAttachmentProject(
    officeApi,
    officeSeed.workspaceId,
    "Office creation attachments",
  );
  const dialog = await openNewTaskDialog(testPage);
  await expect(dialog.getByRole("button", { name: "Attach files" })).toBeVisible();
  await expect(dialog.locator("button:has(svg.tabler-icon-dots-vertical)")).toHaveCount(0);

  await dialog.getByPlaceholder("Task title").fill("Office creation attachment task");
  await chooseProject(testPage, dialog, project.name);

  const upload = waitForHttp(testPage, "POST", /^\/api\/v1\/attachments$/);
  await attachCreationFiles(dialog);
  await upload;
  const create = dialog.getByTestId("new-task-create-button");
  await expect(create).toBeEnabled();

  await dialog.getByRole("button", { name: /^Image \(/ }).click();
  await expect(testPage.getByRole("dialog", { name: "Image preview" })).toBeVisible();
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("dialog", { name: "Image preview" })).toBeHidden();

  await dialog.getByText("logs.zip").click();
  await expect(dialog.getByText(/logs\.zip/).first()).toBeVisible();

  const created = waitForHttp(testPage, "POST", /^\/api\/v1\/tasks$/);
  await create.click();
  const response = await created;
  const body = (await response.json()) as { id?: string };
  expect(body.id).toBeTruthy();
  const requestBody = response.request().postDataJSON() as {
    attachments?: Array<{ attachment_id?: string; name?: string }>;
  };
  expect(requestBody.attachments?.map((attachment) => attachment.name)).toEqual(
    expect.arrayContaining(["screenshot.png", "logs.zip"]),
  );
  expect(requestBody.attachments?.every((attachment) => Boolean(attachment.attachment_id))).toBe(
    true,
  );
});

test("Office creation keeps a ready draft across close and reload", async ({
  testPage,
  officeSeed,
  officeApi,
}) => {
  const project = await createAttachmentProject(
    officeApi,
    officeSeed.workspaceId,
    "Office attachment draft",
  );
  let dialog = await openNewTaskDialog(testPage);
  await dialog.getByPlaceholder("Task title").fill("Draft attachment task");
  await chooseProject(testPage, dialog, project.name);

  await attachCreationFiles(dialog);
  await expect(dialog.getByTestId("new-task-create-button")).toBeEnabled();
  await newTaskDialog(testPage).getByRole("button", { name: "Close" }).click();
  await expect(newTaskDialog(testPage)).toBeHidden();

  await expect
    .poll(() =>
      testPage.evaluate(
        (key) => localStorage.getItem(key) ?? "",
        `kandev:issue-draft:${officeSeed.workspaceId}`,
      ),
    )
    .toContain("screenshot.png");

  await testPage.reload();
  dialog = await openNewTaskDialog(testPage);
  await expect(dialog.getByRole("button", { name: /^Image \(/ })).toBeVisible();
  await expect(dialog.getByText("logs.zip")).toBeVisible();
  await expect(dialog.getByTestId("new-task-create-button")).toBeEnabled();
});

test("Office creation blocks on a failed upload until it is removed", async ({
  testPage,
  officeSeed,
  officeApi,
}) => {
  const project = await createAttachmentProject(
    officeApi,
    officeSeed.workspaceId,
    "Office attachment failure",
  );
  await testPage.route("**/api/v1/attachments", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    const body = route.request().postDataBuffer()?.toString("utf8") ?? "";
    if (body.includes("logs.zip")) {
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "upload rejected" }),
      });
      return;
    }
    await route.continue();
  });

  const dialog = await openNewTaskDialog(testPage);
  await dialog.getByPlaceholder("Task title").fill("Failed attachment task");
  await chooseProject(testPage, dialog, project.name);
  await attachCreationFiles(dialog);

  const create = dialog.getByTestId("new-task-create-button");
  await expect(create).toBeDisabled();
  await dialog.getByText("logs.zip").hover();
  await expect(testPage.getByTestId("attachment-upload-failed")).toBeVisible();
  await expect(testPage.getByTestId("attachment-upload-retry")).toBeVisible();

  await dialog.getByTestId("context-chip-remove").last().click();
  await expect(create).toBeEnabled();
  await expect(dialog.getByText("logs.zip")).toHaveCount(0);
});
test("Office creation persists attachment removal across reload", async ({
  testPage,
  officeSeed,
  officeApi,
}) => {
  const project = await createAttachmentProject(
    officeApi,
    officeSeed.workspaceId,
    "Office attachment removal",
  );
  let dialog = await openNewTaskDialog(testPage);
  await dialog.getByPlaceholder("Task title").fill("Removed attachment task");
  await chooseProject(testPage, dialog, project.name);
  await attachCreationFiles(dialog);
  await expect(dialog.getByTestId("new-task-create-button")).toBeEnabled();

  await dialog
    .getByRole("button", { name: /^Remove Image \(/ })
    .evaluate((element) => (element as HTMLButtonElement).click());
  await expect(dialog.getByRole("button", { name: /^Image \(/ })).toHaveCount(0);
  await expect(dialog.getByText("logs.zip")).toBeVisible();
  await dialog.getByRole("button", { name: "Close" }).click();
  await expect(newTaskDialog(testPage)).toBeHidden();

  await expect
    .poll(() =>
      testPage.evaluate(
        (key) => localStorage.getItem(key) ?? "",
        `kandev:issue-draft:${officeSeed.workspaceId}`,
      ),
    )
    .toContain("logs.zip");
  const savedDraft = await testPage.evaluate(
    (key) => localStorage.getItem(key) ?? "",
    `kandev:issue-draft:${officeSeed.workspaceId}`,
  );
  expect(savedDraft).not.toContain("screenshot.png");

  await testPage.reload();
  dialog = await openNewTaskDialog(testPage);
  await expect(dialog.getByText("logs.zip")).toBeVisible();
  await expect(dialog.getByRole("button", { name: /^Image \(/ })).toHaveCount(0);
});
