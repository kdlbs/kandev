import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { exerciseChoiceImageQuestion } from "./choice-image-helpers";
import {
  exerciseImageCollapse,
  inspectAndAnswerImages,
  openImageQuestions,
  seedQuestionImage,
  previewName,
} from "./question-image-helpers";

test("choice images inspect without selecting and survive reload", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180_000);
  await exerciseChoiceImageQuestion(testPage, apiClient, seedData, false, prCapture);
});

test("inspects images and submits one keyed question bundle after reload", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
  backend,
}) => {
  test.setTimeout(180_000);
  seedQuestionImage(seedData.repositoryPath);
  const { task, session } = await openImageQuestions(testPage, apiClient, seedData);
  await testPage.reload();
  await expect(
    session.clarificationOverlay().getByRole("img", { name: "Proposed layout", exact: true }),
  ).toBeVisible();
  await expect(
    session.clarificationOverlay().getByText("Approve this layout?", { exact: true }),
  ).toBeInViewport({ ratio: 1 });
  const thumbnail = await session
    .clarificationOverlay()
    .getByRole("img", { name: "Proposed layout", exact: true })
    .boundingBox();
  expect(thumbnail!.height).toBeLessThanOrEqual(128);
  await prCapture.screenshot("desktop-question-images", {
    caption: "Workspace image beside a blocking question",
  });
  if (prCapture.capturing) {
    await session
      .clarificationOverlay()
      .getByTestId("clarification-input")
      .scrollIntoViewIfNeeded();
    await prCapture.screenshot("desktop-question-answer-controls", {
      caption: "Clarification question with its image, choices, and text answer",
    });
  }
  await inspectAndAnswerImages(testPage, session, false, () =>
    prCapture.screenshot("desktop-question-image-viewer", {
      caption: "Image inspection preserves the answer draft",
    }),
  );
  const { messages } = await apiClient.listSessionMessages(task.session_id!);
  const questions = messages.filter((message) => message.type === "clarification_request");
  expect(questions).toHaveLength(3);
  expect(questions.every((message) => message.metadata?.status === "answered")).toBe(true);
  const copy = questions.find((message) => message.metadata?.question_id === "copy");
  expect(copy?.metadata?.response).toMatchObject({ custom_text: "Shorten the heading." });
  const history = testPage
    .getByTestId("clarification-request-message")
    .filter({ hasText: "Approve this layout?" });
  await expect(history.getByRole("img", { name: "Proposed layout", exact: true })).toHaveCount(0);
  await history.getByRole("button", { name: "Show images", exact: true }).click();
  await expect(history.getByRole("img", { name: "Proposed layout", exact: true })).toBeVisible();

  const changedImage = await testPage.evaluate(() => {
    const canvas = document.createElement("canvas");
    canvas.width = 800;
    canvas.height = 2400;
    const context = canvas.getContext("2d")!;
    context.fillStyle = "#845ec2";
    context.fillRect(0, 0, canvas.width, canvas.height);
    return canvas.toDataURL("image/png").split(",")[1];
  });
  fs.writeFileSync(
    path.join(seedData.repositoryPath, previewName),
    Buffer.from(changedImage, "base64"),
  );
  await backend.restart();
  await testPage.reload();
  await expect(history.getByRole("img", { name: "Proposed layout", exact: true })).toHaveCount(0);
  await history.getByRole("button", { name: "Show images", exact: true }).click();
  await expect(history.getByRole("img", { name: "Proposed layout", exact: true })).toBeVisible({
    timeout: 30_000,
  });
  await expect
    .poll(() =>
      history
        .getByRole("img", { name: "Proposed layout", exact: true })
        .evaluate((image) => (image as HTMLImageElement).naturalHeight),
    )
    .toBe(2400);
  await history.getByRole("button", { name: /Proposed layout/ }).click();
  const historyViewer = testPage.getByRole("dialog", { name: "Proposed layout" });
  await historyViewer.getByRole("button", { name: "Actual size", exact: true }).click();
  const canvas = historyViewer.getByRole("region", { name: "Image inspection" });
  await expect
    .poll(() => canvas.evaluate((element) => element.scrollHeight > element.clientHeight))
    .toBe(true);
  await historyViewer.press("Escape");
  const restored = await apiClient.listSessionMessages(task.session_id!);
  expect(
    restored.messages.find((message) => message.id === questions[0].id)?.metadata?.question,
  ).toEqual(questions[0].metadata?.question);
  fs.unlinkSync(path.join(seedData.repositoryPath, previewName));
  await history.getByRole("button", { name: "Hide images", exact: true }).click();
  await history.getByRole("button", { name: "Show images", exact: true }).click();
  await expect(history).toContainText("Image file not found.");
});

test("inspection keeps a draft across the phone breakpoint and theme changes", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  seedQuestionImage(seedData.repositoryPath);
  const { session } = await openImageQuestions(testPage, apiClient, seedData);
  const overlay = session.clarificationOverlay();
  await session.clarificationOption("Approve").click();
  await expect(overlay).toContainText("Does this text need changes?");
  const draft = overlay.getByTestId("clarification-input");
  await draft.fill("Preserve this draft.");
  await testPage.emulateMedia({ reducedMotion: "reduce" });
  for (const width of [767, 768, 1280]) {
    await testPage.setViewportSize({ width, height: 850 });
    await expect(draft).toHaveValue("Preserve this draft.");
    await expect(overlay).toContainText("Does this text need changes?");
    await expect(overlay).toContainText("2 of 3 answered");
    const inspect = overlay.getByRole("button", { name: /Proposed layout/ });
    const thumbnail = await inspect.getByRole("img").boundingBox();
    expect(thumbnail!.height).toBeLessThanOrEqual(width < 768 ? 96 : 128);
    if (width < 768) {
      const target = await inspect.boundingBox();
      expect(target!.height).toBeGreaterThanOrEqual(44);
      expect(target!.width).toBeGreaterThanOrEqual(44);
    }
    await inspect.click();
    const viewer = testPage.getByRole("dialog", { name: "Proposed layout" });
    await expect(viewer).toBeVisible();
    const box = await viewer.boundingBox();
    if (width < 768) expect(box!.height).toBeGreaterThanOrEqual(849);
    else expect(box!.height).toBeLessThan(850);
    await testPage.evaluate(() => document.documentElement.classList.add("dark"));
    if (prCapture.capturing) await waitForFiniteAnimations(testPage.locator("html"));
    await prCapture.screenshot(`question-image-dark-${width}`);
    await viewer.press("Escape");
    await expect(inspect).toBeFocused();
    await expect(draft).toHaveValue("Preserve this draft.");
  }
  await session.clarificationSkip().click();
  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
});

test("desktop collapse releases previews and preserves the answer draft", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  seedQuestionImage(seedData.repositoryPath);
  const { session } = await openImageQuestions(testPage, apiClient, seedData);
  await exerciseImageCollapse(testPage, session, false);
  if (prCapture.capturing) {
    await testPage.setViewportSize({ width: 1280, height: 1000 });
    // Redact the local shell prompt from this question-only capture.
    await testPage.addStyleTag({ content: ".xterm-screen { visibility: hidden; }" });
  }
  await prCapture.screenshot("desktop-question-images-reopened", {
    caption: "Reopened clarification reloads its image and keeps the answer draft",
  });
  const skipped = waitForHttp(testPage, "POST", /\/api\/v1\/clarification\/[^/]+\/respond$/, {
    predicate: (response) => response.ok(),
  });
  await session.clarificationSkip().click();
  await skipped;
  await expect(session.idleInput()).toBeVisible();
});
