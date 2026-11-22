import fs from "node:fs";
import path from "node:path";
import { expect, type Locator, type Page } from "@playwright/test";
import type { OfficeApiClient } from "../../helpers/office-api-client";

export function newTaskDialog(page: Page): Locator {
  return page.getByTestId("office-new-issue-dialog");
}

export async function openNewTaskDialog(page: Page, mobile = false) {
  await page.goto("/office/tasks");
  if (mobile) {
    await page.getByRole("main").getByRole("button", { name: "New Task", exact: true }).tap();
  } else {
    await page.locator('button:has(svg.tabler-icon-plus):has-text("New Task")').first().click();
  }
  const dialog = newTaskDialog(page);
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

export async function createAttachmentProject(
  officeApi: OfficeApiClient,
  workspaceId: string,
  label: string,
) {
  const project = (await officeApi.createProject(workspaceId, `${label} ${Date.now()}`)) as {
    id: string;
    name: string;
  };
  expect(project.id).toBeTruthy();
  return project;
}

export async function chooseProject(page: Page, dialog: Locator, projectName: string) {
  await dialog.getByRole("button", { name: "Project" }).click();
  await page.getByRole("button", { name: projectName, exact: true }).last().click();
  await dialog.getByPlaceholder("Task title").click();
}

export function imageBytes(): Buffer {
  return fs.readFileSync(path.join(process.cwd(), "public/web-app-manifest-192x192.png"));
}

export async function attachCreationFiles(dialog: Locator) {
  await dialog.locator('input[type="file"]').setInputFiles([
    { name: "screenshot.png", mimeType: "image/png", buffer: imageBytes() },
    { name: "logs.zip", mimeType: "application/zip", buffer: Buffer.from("diagnostic logs") },
  ]);
  await expect(dialog.getByRole("button", { name: /^Image \(/ })).toBeVisible();
  await expect(dialog.getByText("logs.zip")).toBeVisible();
}

export async function pasteReadableImage(dialog: Locator) {
  const bytes = Array.from(imageBytes());
  await dialog.getByTestId("task-description-input").evaluate((element, imageData) => {
    const image = new File([new Uint8Array(imageData)], "pasted.png", { type: "image/png" });
    const clipboardData = new DataTransfer();
    clipboardData.items.add(image);
    const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
    element.dispatchEvent(pasteEvent);
  }, bytes);
  await expect(dialog.getByRole("button", { name: /^Image \(/ })).toHaveCount(2);
}
