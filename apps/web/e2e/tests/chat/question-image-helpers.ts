import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

export const previewName = "question-preview.png";
export function questionImageScript() {
  const image = { id: "layout", path: previewName, alt: "Proposed layout" };
  const questions = [
    { id: "layout", prompt: "Approve this layout?", images: [image] },
    { id: "copy", prompt: "Does this text need changes?", images: [image] },
    {
      id: "missing",
      prompt: "Continue without the missing image?",
      images: [{ id: "missing", path: "missing.png", alt: "Missing screenshot" }],
    },
  ].map((question) => ({
    ...question,
    options: [
      { label: "Approve", description: "Accept" },
      { label: "Revise", description: "Request changes" },
    ],
  }));
  return `e2e:mcp:kandev:ask_user_question_kandev(${JSON.stringify({ questions })})\ne2e:message("Image questions complete.")`;
}

export function seedQuestionImage(repositoryPath: string) {
  fs.copyFileSync(
    path.resolve(__dirname, "../../fixtures/rich-output-image.png"),
    path.join(repositoryPath, previewName),
  );
}

export function questionImageSetupScript() {
  const base64 = fs
    .readFileSync(path.resolve(__dirname, "../../fixtures/rich-output-image.png"))
    .toString("base64");
  return `printf '%s' '${base64}' | base64 -d > ${previewName}`;
}

type Seed = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  agentProfileId: string;
  repositoryId: string;
};
export async function openImageQuestions(
  page: Page,
  api: ApiClient,
  seed: Seed,
  executor?: string,
  script = questionImageScript(),
) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Image questions",
    seed.agentProfileId,
    {
      description: script,
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      ...(executor ? { executor_profile_id: executor } : {}),
    },
  );
  if (!task.session_id) throw new Error("Image fixture has no session");
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "Image question must block its source session",
    timeout: 90_000,
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(
    session
      .clarificationOverlay()
      .getByRole("img", { name: "Proposed layout", exact: true })
      .first(),
  ).toBeVisible({ timeout: 30_000 });
  return { task, session };
}

export async function inspectAndAnswerImages(
  page: Page,
  session: SessionPage,
  mobile: boolean,
  captureViewer?: () => Promise<unknown>,
) {
  const overlay = session.clarificationOverlay();
  const draft = overlay.getByTestId("clarification-input");
  await draft.fill("Keep the spacing.");
  const inspect = overlay.getByRole("button", { name: /Proposed layout/ });
  await inspect.click();
  const viewer = page.getByRole("dialog", { name: "Proposed layout" });
  await expect(viewer).toBeVisible();
  await captureViewer?.();
  await viewer.getByRole("button", { name: "Actual size", exact: true }).click();
  await expect(viewer.getByRole("button", { name: "Actual size", exact: true })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await viewer.press("ArrowRight");
  if (mobile) {
    const box = await viewer.boundingBox();
    expect(box?.height).toBeGreaterThan(page.viewportSize()!.height - 3);
    const close = viewer.getByRole("button", { name: "Close", exact: true });
    expect((await close.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await close.tap();
  } else {
    await viewer.press("Escape");
    await expect(inspect).toBeFocused();
  }
  await expect(viewer).toHaveCount(0);
  await expect(draft).toHaveValue("Keep the spacing.");
  await session.clarificationOption("Approve").click();
  await expect(overlay).toContainText("Does this text need changes?");
  await expect(overlay.getByRole("img", { name: "Proposed layout", exact: true })).toBeVisible();
  await overlay.getByTestId("clarification-input").fill("Shorten the heading.");
  await overlay.getByTestId("clarification-next").click();
  await expect(overlay).toContainText("Image file not found.");
  await session.clarificationOption("Revise").click();
  await overlay.getByRole("button", { name: /^Submit/ }).click();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  await expect(session.chat).toContainText("Image questions complete.");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
