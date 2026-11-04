import { createHash } from "node:crypto";
import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";
import { seedClarificationSession } from "../../helpers/clarification";

const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMB/6X3pZQAAAAASUVORK5CYII=";
const PNG_BYTES = Buffer.from(PNG_BASE64, "base64");
const PNG_SHA = createHash("sha256").update(PNG_BYTES).digest("hex");
const MIN_TOUCH_TARGET_PX = 44;

async function expectPhoneManagerTouchTargets(manager: Locator) {
  const [addBox, removeBox, inspectBox] = await Promise.all([
    manager.getByTestId("clarification-attachment-add").boundingBox(),
    manager.getByTestId("clarification-attachment-remove").boundingBox(),
    manager.getByTestId("clarification-attachment-inspect").boundingBox(),
  ]);
  for (const box of [addBox, removeBox, inspectBox]) {
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(box?.width ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
  }
}

async function expectPhoneTouchTarget(locator: Locator) {
  const box = await locator.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
  expect(box?.width ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
}

test.describe("Mobile question answer attachments", () => {
  test.describe.configure({ timeout: 120_000 });

  test("a choice-only answer uses header attachment and submit controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Choice Only Answer Files",
      { scenario: "clarification-files-choice" },
    );
    const overlay = session.clarificationOverlay();
    await expect(overlay).toBeVisible({ timeout: 30_000 });
    await expect(overlay.getByTestId("clarification-custom-input")).toHaveCount(0);

    const trigger = session.clarificationAttachmentTrigger();
    const triggerBox = await trigger.boundingBox();
    expect(triggerBox?.width ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(triggerBox?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);

    await session.clarificationOption("Fast").tap();
    await expect(session.idleInput()).toHaveCount(0);
    const upload = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    );
    await session.clarificationAttachmentInput().setInputFiles({
      name: "mobile.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    expect((await upload).ok()).toBe(true);
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");

    await trigger.tap();
    await expect(session.clarificationAttachmentManager()).toBeVisible();
    await testPage.keyboard.press("Escape");

    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText(
      `Attachment proof: mobile.png ${PNG_BYTES.length} ${PNG_SHA}`,
      { timeout: 30_000 },
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });

  test("the paperclip stays on the right of the custom answer row", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Custom Answer Files",
      { scenario: "clarification-files" },
    );
    const overlay = session.clarificationOverlay();
    await expect(overlay).toBeVisible({ timeout: 30_000 });
    await session.clarificationOption("MongoDB").tap();

    const inputBox = await session.clarificationInput().boundingBox();
    const triggerBox = await session.clarificationAttachmentTrigger().boundingBox();
    const emptyRowBox = await session.clarificationCustomInput().boundingBox();
    expect(inputBox).not.toBeNull();
    expect(triggerBox).not.toBeNull();
    expect(emptyRowBox).not.toBeNull();
    expect((triggerBox?.x ?? 0) > (inputBox?.x ?? 0)).toBe(true);
    expect(triggerBox?.height ?? 0).toBeGreaterThanOrEqual(MIN_TOUCH_TARGET_PX);
    expect(emptyRowBox?.height ?? 0).toBeLessThanOrEqual(48);

    const upload = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" && response.url().includes("/api/v1/attachments"),
    );
    await session.clarificationAttachmentInput().setInputFiles({
      name: "row.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    await upload;
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");
    const readyRowBox = await session.clarificationCustomInput().boundingBox();
    expect(readyRowBox?.height ?? 0).toBeCloseTo(emptyRowBox?.height ?? 0, 0);
    expect(readyRowBox?.height ?? 0).toBeLessThanOrEqual(48);

    await session.clarificationAttachmentTrigger().tap();
    const manager = session.clarificationAttachmentManager();
    await expect(manager).toBeVisible();
    await expectPhoneManagerTouchTargets(manager);
    await testPage.keyboard.press("Escape");

    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("Attachment proof: row.png", {
      timeout: 30_000,
    });
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });

  test("retry in the phone attachment manager meets the touch contract", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Retry Answer Files",
      { scenario: "clarification-files" },
    );
    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await session.clarificationOption("SQLite").tap();
    const emptyHeight = (await session.clarificationCustomInput().boundingBox())?.height ?? 0;

    let releaseFailedUpload!: () => void;
    const holdFailedUpload = new Promise<void>((resolve) => {
      releaseFailedUpload = resolve;
    });
    let failedOnce = false;
    await testPage.route("**/api/v1/attachments", async (route) => {
      if (!failedOnce && route.request().method() === "POST") {
        failedOnce = true;
        await holdFailedUpload;
        await route.fulfill({ status: 500, body: "upload failed" });
        return;
      }
      await route.continue();
    });
    await session.clarificationAttachmentInput().setInputFiles({
      name: "mobile-retry.png",
      mimeType: "image/png",
      buffer: PNG_BYTES,
    });
    await expect(session.clarificationAttachmentBadge()).toHaveAttribute("data-state", "uploading");
    const uploadingHeight = (await session.clarificationCustomInput().boundingBox())?.height ?? 0;
    releaseFailedUpload();
    await expect(session.clarificationAttachmentBadge()).toHaveAttribute("data-state", "failed");
    const failedHeight = (await session.clarificationCustomInput().boundingBox())?.height ?? 0;
    expect(uploadingHeight).toBeCloseTo(emptyHeight, 0);
    expect(failedHeight).toBeCloseTo(emptyHeight, 0);
    expect(emptyHeight).toBeLessThanOrEqual(48);
    await session.clarificationAttachmentTrigger().tap();
    const manager = session.clarificationAttachmentManager();
    await expect(manager).toBeVisible();
    await expectPhoneTouchTarget(manager.getByTestId("clarification-attachment-retry"));

    const retryUpload = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().includes("/api/v1/attachments") &&
        response.ok(),
    );
    await manager.getByTestId("clarification-attachment-retry").tap();
    await retryUpload;
    await expect(session.clarificationAttachmentBadge()).toHaveText("1");
    const readyHeight = (await session.clarificationCustomInput().boundingBox())?.height ?? 0;
    expect(readyHeight).toBeCloseTo(emptyHeight, 0);
    await testPage.keyboard.press("Escape");
    await session.submitClarification();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("Attachment proof: mobile-retry.png", {
      timeout: 30_000,
    });
  });
});
