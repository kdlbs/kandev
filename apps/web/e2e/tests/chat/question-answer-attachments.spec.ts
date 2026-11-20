import { createHash } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import { seedClarificationSession } from "../../helpers/clarification";
import { pastePng } from "./composer-attachment-scope-helpers";

const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/6X3pZQAAAAASUVORK5CYII=";
const PNG_BYTES = Buffer.from(PNG_BASE64, "base64");
const PNG_SHA = createHash("sha256").update(PNG_BYTES).digest("hex");

test.describe("question answer attachments", () => {
  test("a choice-only answer accepts a file and requires explicit submit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Choice only answer attachment",
      { scenario: "clarification-files-choice" },
    );
    const overlay = session.clarificationOverlay();
    await expect(overlay).toBeVisible({ timeout: 30_000 });
    await expect(overlay.getByTestId("clarification-custom-input")).toHaveCount(0);

    await session.clarificationOption("Fast").click();
    await expect(session.idleInput()).toHaveCount(0);

    const upload = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    );
    await session.clarificationAttachmentInput().setInputFiles({
      name: "evidence.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    expect((await upload).ok()).toBe(true);
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");

    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText(
      `Attachment proof: evidence.png ${PNG_BYTES.length} ${PNG_SHA}`,
      { timeout: 30_000 },
    );
  });

  test("an image pasted into a custom answer preserves the selected option", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Pasted answer attachment",
      { scenario: "clarification-files" },
    );
    const overlay = session.clarificationOverlay();
    await expect(overlay).toBeVisible({ timeout: 30_000 });

    await session.clarificationOption("PostgreSQL").click();
    await expect(session.clarificationOption("PostgreSQL")).toHaveAttribute(
      "data-selected",
      "true",
    );
    const upload = await pastePng(testPage, session.clarificationInput(), "pasted.png");
    expect(upload.ok()).toBe(true);
    await expect(session.clarificationOption("PostgreSQL")).toHaveAttribute(
      "data-selected",
      "true",
    );
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");

    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("Attachment proof: pasted.png", {
      timeout: 30_000,
    });
  });

  test("a failed upload can be retried from the attachment manager", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Retried answer attachment",
      { scenario: "clarification-files" },
    );
    const overlay = session.clarificationOverlay();
    await expect(overlay).toBeVisible({ timeout: 30_000 });
    await session.clarificationOption("SQLite").click();

    let failedOnce = false;
    await testPage.route("**/api/v1/attachments", async (route) => {
      if (!failedOnce && route.request().method() === "POST") {
        failedOnce = true;
        await route.fulfill({ status: 500, body: "upload failed" });
        return;
      }
      await route.continue();
    });
    await session.clarificationAttachmentInput().setInputFiles({
      name: "retry.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    await expect(session.clarificationAttachmentBadge()).toHaveAttribute("data-state", "failed");

    await session.clarificationAttachmentTrigger().click();
    const manager = session.clarificationAttachmentManager();
    await expect(manager).toBeVisible();
    const retryUpload = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().includes("/api/v1/attachments") &&
        response.ok(),
    );
    await manager.getByTestId("clarification-attachment-retry").click();
    await retryUpload;
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");

    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("Attachment proof: retry.png", {
      timeout: 30_000,
    });
  });

  test("a custom-text answer carries its attached file", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Custom answer attachment",
      { scenario: "clarification-files" },
    );
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await session.clarificationInput().fill("Use SQLite for the local cache");

    const upload = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    );
    await session.clarificationAttachmentInput().setInputFiles({
      name: "custom.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    expect((await upload).ok()).toBe(true);
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");

    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("custom.png", { timeout: 30_000 });
    await expect(session.chat).toContainText("Attachment proof: custom.png", {
      timeout: 30_000,
    });
  });

  test("keeps files with their question across a multi-question bundle", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Multi question answer attachment",
      { scenario: "clarification-multi" },
    );
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const uploadFirst = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    );
    await session.clarificationAttachmentInput().setInputFiles({
      name: "multi-db.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    expect((await uploadFirst).ok()).toBe(true);
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");
    await session.clarificationAttachmentTrigger().click();
    await expect(session.clarificationAttachmentManager()).toContainText("multi-db.png");
    await testPage.keyboard.press("Escape");
    await session.clarificationOption("PostgreSQL").click();

    const uploadSecond = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    );
    await session.clarificationAttachmentInput().setInputFiles({
      name: "multi-language.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    expect((await uploadSecond).ok()).toBe(true);
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");
    await session.clarificationAttachmentTrigger().click();
    await expect(session.clarificationAttachmentManager()).toContainText("multi-language.png");
    await expect(session.clarificationAttachmentManager()).not.toContainText("multi-db.png");
    await testPage.keyboard.press("Escape");
    await session.clarificationOption("Go").click();
    await session.clarificationOption("Docker").click();

    const submit = session.clarificationSubmit();
    await expect(submit).toBeEnabled();
    await submit.click();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("multi-db.png", { timeout: 30_000 });
    await expect(session.chat).toContainText("multi-language.png", { timeout: 30_000 });
    await expect(session.chat).toContainText("/.kandev/attachments/", { timeout: 30_000 });
  });

  test("a narrow fine-pointer viewport uses the phone attachment presentation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    await testPage.setViewportSize({ width: 500, height: 800 });
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Narrow fine pointer answer files",
      { scenario: "clarification-files" },
    );
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    const triggerBox = await session.clarificationAttachmentTrigger().boundingBox();
    expect(triggerBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    expect(triggerBox?.width ?? 0).toBeGreaterThanOrEqual(44);
    await session.clarificationAttachmentTrigger().click();
    await expect(session.clarificationAttachmentManager()).toBeVisible();
    await testPage.keyboard.press("Escape");
  });
});
