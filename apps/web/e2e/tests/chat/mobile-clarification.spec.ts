import { test, expect } from "../../fixtures/test-base";
import { seedClarificationSession } from "../../helpers/clarification";

/**
 * Mobile parity for the multiline custom clarification answer. On a coarse-pointer
 * device Enter inserts a newline instead of submitting, and the send affordance is
 * the inline "Send" button (there is no overlay Submit button for single-question
 * bundles). Runs under the Pixel 5 `mobile-chrome` project.
 */
test.describe("Mobile clarification multiline answer", () => {
  test.describe.configure({ timeout: 120_000 });

  test("Auto-run ON does not bypass a pending clarification on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify Composer Queue",
      { scenario: "clarification" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    const composer = session.activeChat().getByTestId("chat-input-editor");
    await expect(composer).toHaveAttribute("contenteditable", "true", { timeout: 30_000 });
    await composer.pressSequentially("Queue this from phone 1", { timeout: 30_000 });
    await expect(composer).toContainText("Queue this from phone 1");
    await expect(session.clarificationOverlay()).toBeVisible();
    await testPage.getByTestId("submit-message-button").tap();

    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
    await expect(session.clarificationOverlay()).toBeVisible();
    await testPage.getByTestId("queue-chip").tap();
    const panel = testPage.getByTestId("queued-ghost-list");
    await expect(panel).toBeVisible();
    const autoRun = panel.getByTestId("queue-auto-run");
    await expect(autoRun).toHaveAttribute("data-state", "checked");
    await autoRun.tap();
    await expect(autoRun).toHaveAttribute("data-state", "unchecked");
    await autoRun.tap();
    await expect(autoRun).toHaveAttribute("data-state", "checked");
    await expect(panel.getByTestId("queue-entry")).toHaveCount(1);
    await expect(session.clarificationOverlay()).toBeVisible();
  });

  test("Enter inserts a newline and the Send button submits the multiline answer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify",
      {
        scenario: "clarification",
      },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const input = session.clarificationInput();
    // Tap the apparent row surface, outside the textarea itself. The whole row
    // is the mobile touch target and should transfer focus into the textarea.
    await session.clarificationCustomInput().tap({ position: { x: 4, y: 4 } });
    await expect(input).toBeFocused();
    await input.pressSequentially("first line");
    // On touch, Enter inserts a newline rather than submitting.
    await input.press("Enter");
    await input.pressSequentially("second line");
    await expect(input).toHaveValue("first line\nsecond line");
    // The overlay is still open — Enter did not submit.
    await expect(session.clarificationOverlay()).toBeVisible();

    // The inline Send button is the touch send affordance.
    await expect(session.clarificationCustomSubmit()).toBeVisible();
    await session.clarificationCustomSubmit().tap();

    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(session.chat).toContainText("first line");
    await expect(session.chat).toContainText("second line");
    await expect(session.chat).not.toContainText("linesecond line");
  });

  test("shows the header submitting status for a single-question answer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify Submit Feedback",
      { scenario: "clarification" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    const overlay = session.clarificationOverlay();
    const header = overlay.getByTestId("clarification-overlay-header");
    const status = session.clarificationSubmittingStatus();
    await expect(status).toHaveCount(0);

    let releaseResponse = () => undefined;
    const heldResponse = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });
    await testPage.route("**/api/v1/clarification/*/respond", async (route) => {
      await heldResponse;
      await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
    });

    try {
      await session.clarificationOption("PostgreSQL").tap();
      await expect(status).toBeVisible();
      await expect(status).toHaveAttribute("aria-label", "Submitting…");
      await expect(session.clarificationSkip()).toBeDisabled();

      const [headerBox, statusBox, skipBox, collapseBox] = await Promise.all([
        header.boundingBox(),
        status.boundingBox(),
        session.clarificationSkip().boundingBox(),
        session.clarificationCollapseToggle().boundingBox(),
      ]);
      if (!headerBox || !statusBox || !skipBox || !collapseBox) {
        throw new Error("expected mobile clarification header controls to have bounding boxes");
      }

      expect(statusBox.x).toBeGreaterThanOrEqual(headerBox.x);
      expect(statusBox.x + statusBox.width).toBeLessThanOrEqual(headerBox.x + headerBox.width);
      expect(skipBox.height).toBeGreaterThanOrEqual(44);
      expect(skipBox.width).toBeGreaterThanOrEqual(44);
      expect(collapseBox.height).toBeGreaterThanOrEqual(44);
      expect(collapseBox.width).toBeGreaterThanOrEqual(44);

      const statusPrecedesSkip = await header.evaluate((node) => {
        const submitting = node.querySelector('[data-testid="clarification-submitting-status"]');
        const skip = node.querySelector('[data-testid="clarification-skip"]');
        return Boolean(
          submitting &&
          skip &&
          submitting.compareDocumentPosition(skip) & Node.DOCUMENT_POSITION_FOLLOWING,
        );
      });
      expect(statusPrecedesSkip).toBe(true);
      await expect(
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).resolves.toBe(true);
    } finally {
      releaseResponse();
    }

    await expect(overlay).not.toBeVisible({ timeout: 30_000 });
  });

  test("keeps the over-limit counter inside the phone viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify Limit",
      { scenario: "clarification" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await session.clarificationInput().fill("a".repeat(2001));

    const counter = session.clarificationOverlay().getByTestId("clarification-input-rune-counter");
    await expect(counter).toHaveAttribute("data-over-limit", "true");

    const [counterBox, overlayBox] = await Promise.all([
      counter.boundingBox(),
      session.clarificationOverlay().boundingBox(),
    ]);
    if (!counterBox || !overlayBox) {
      throw new Error("expected mobile counter and overlay to have bounding boxes");
    }
    expect(counterBox.x).toBeGreaterThanOrEqual(overlayBox.x - 1);
    expect(counterBox.x + counterBox.width).toBeLessThanOrEqual(
      overlayBox.x + overlayBox.width + 1,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });

  test("shows shared context once above the question on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify Shared Context",
      { scenario: "clarification-multi" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });

    const context = session.clarificationContext();
    await expect(context).toHaveCount(1);
    await expect(context).toHaveText(
      "Picking the foundational stack.\n\nAnswer all three so we can move forward.",
    );
    await expect(context).not.toContainText(String.raw`\n`);
    await expect(context).toHaveCSS("font-size", "13px");
    await expect(context).toHaveCSS("margin-top", "12px");
    await expect(context).toHaveCSS("padding", "0px");
    await expect(context).toHaveCSS("border-width", "0px");
    await expect(context).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
    await expect(
      session.clarificationQuestionCards().getByTestId("clarification-context"),
    ).toHaveCount(0);

    const [contextBox, overlayBox] = await Promise.all([
      context.boundingBox(),
      session.clarificationOverlay().boundingBox(),
    ]);
    if (!contextBox || !overlayBox) {
      throw new Error("expected mobile shared context and overlay to have bounding boxes");
    }
    expect(contextBox.x).toBeGreaterThanOrEqual(overlayBox.x - 1);
    expect(contextBox.x + contextBox.width).toBeLessThanOrEqual(
      overlayBox.x + overlayBox.width + 1,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await session.clarificationStep(1).tap();
    await expect(session.clarificationStep(1)).toHaveAttribute("data-active", "true");
    await expect(context).toHaveCount(1);
  });

  test("renders lightweight markdown without overflow and submits through formatted content", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarification Markdown",
      { scenario: "clarification-markdown" },
    );

    const overlay = session.clarificationOverlay();
    await expect(overlay).toBeVisible();
    const card = session.clarificationQuestionCardById("markdown");
    const option = session.clarificationOption("Postgres");

    await expect(card.getByTestId("clarification-question-title").locator("code")).toHaveText("DB");
    await expect(card.locator("ol > li")).toHaveCount(2);
    await expect(
      option.getByTestId("clarification-option-description").locator("strong"),
    ).toHaveText("production");
    await expect(option.locator("a")).toHaveCount(0);

    await option.scrollIntoViewIfNeeded();
    const [optionBox, overlayBox] = await Promise.all([
      option.boundingBox(),
      overlay.boundingBox(),
    ]);
    if (!optionBox || !overlayBox) {
      throw new Error("expected formatted mobile option and overlay to have bounding boxes");
    }
    expect(optionBox.x).toBeGreaterThanOrEqual(overlayBox.x - 1);
    expect(optionBox.x + optionBox.width).toBeLessThanOrEqual(overlayBox.x + overlayBox.width + 1);
    await expect(
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).resolves.toBe(true);

    await option.locator("code").tap();
    await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
    await expect(
      session
        .activeChat()
        .getByTestId("clarification-request-message")
        .locator("code")
        .filter({ hasText: "Postgres" }),
    ).toBeVisible();
    await expect(
      testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).resolves.toBe(true);
  });

  test("separates batch actions from the stepper while showing submission feedback", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedClarificationSession(
      testPage,
      apiClient,
      seedData,
      "Mobile Clarify Submit Feedback",
      { scenario: "clarification-multi" },
    );

    await expect(session.clarificationOverlay()).toBeVisible({ timeout: 30_000 });
    await session.clarificationOption("PostgreSQL").tap();
    await session.clarificationOption("Go").tap();
    await session.clarificationOption("Docker").tap();

    let releaseResponse = () => undefined;
    const heldResponse = new Promise<void>((resolve) => {
      releaseResponse = resolve;
    });
    await testPage.route("**/api/v1/clarification/*/respond", async (route) => {
      await heldResponse;
      await route.fulfill({ status: 200, contentType: "application/json", body: "{}" });
    });

    const header = session.clarificationOverlay().getByTestId("clarification-overlay-header");
    const stepper = session.clarificationOverlay().getByTestId("clarification-stepper");
    const submit = session.clarificationSubmit();
    const skip = session.clarificationSkip();
    const collapse = session.clarificationCollapseToggle();
    await expect(submit).toBeEnabled();
    const [headerBox, stepperBox, idleSubmitBox, skipBox, collapseBox] = await Promise.all([
      header.boundingBox(),
      stepper.boundingBox(),
      submit.boundingBox(),
      skip.boundingBox(),
      collapse.boundingBox(),
    ]);
    if (!headerBox || !stepperBox || !idleSubmitBox || !skipBox || !collapseBox) {
      throw new Error("expected mobile clarification header controls to have bounding boxes");
    }

    expect(idleSubmitBox.y).toBeGreaterThanOrEqual(stepperBox.y + stepperBox.height + 4);
    expect(skipBox.height).toBeGreaterThanOrEqual(44);
    expect(skipBox.width).toBeGreaterThanOrEqual(44);
    expect(collapseBox.height).toBeGreaterThanOrEqual(44);
    expect(collapseBox.width).toBeGreaterThanOrEqual(44);
    expect(idleSubmitBox.x).toBeGreaterThanOrEqual(headerBox.x);
    expect(collapseBox.x + collapseBox.width).toBeLessThanOrEqual(headerBox.x + headerBox.width);
    try {
      await submit.tap();
      await expect(submit).toContainText("Submitting");
      await expect(submit).toBeDisabled();
      const status = session.clarificationSubmittingStatus();
      await expect(status).toBeVisible();
      await expect(status).toHaveAttribute("aria-label", "Submitting…");
      await expect(status).not.toHaveAttribute("aria-hidden");
      await expect(submit).toHaveAttribute("aria-label", "Submit");
      await expect(submit.locator('[role="status"]')).toHaveCount(0);
      await expect(submit.locator("svg.tabler-icon-check")).toHaveCount(0);
      const statusPrecedesSkip = await header.evaluate((node) => {
        const submitting = node.querySelector('[data-testid="clarification-submitting-status"]');
        const headerSkip = node.querySelector('[data-testid="clarification-skip"]');
        return Boolean(
          submitting &&
          headerSkip &&
          submitting.compareDocumentPosition(headerSkip) & Node.DOCUMENT_POSITION_FOLLOWING,
        );
      });
      expect(statusPrecedesSkip).toBe(true);
      const [pendingSubmitBox, pendingSkipBox, pendingCollapseBox] = await Promise.all([
        submit.boundingBox(),
        skip.boundingBox(),
        collapse.boundingBox(),
      ]);
      if (!pendingSubmitBox || !pendingSkipBox || !pendingCollapseBox) {
        throw new Error("expected pending mobile clarification controls to have bounding boxes");
      }
      expect(pendingSkipBox.height).toBeGreaterThanOrEqual(44);
      expect(pendingSkipBox.width).toBeGreaterThanOrEqual(44);
      expect(pendingCollapseBox.height).toBeGreaterThanOrEqual(44);
      expect(pendingCollapseBox.width).toBeGreaterThanOrEqual(44);
      await expect(
        testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).resolves.toBe(true);

      expect(idleSubmitBox.height).toBeGreaterThanOrEqual(44);
      expect(pendingSubmitBox.height).toBeGreaterThanOrEqual(44);
      expect(Math.abs(pendingSubmitBox.height - idleSubmitBox.height)).toBeLessThanOrEqual(1);
    } finally {
      releaseResponse();
    }
    await expect(session.clarificationOverlay()).not.toBeVisible({ timeout: 30_000 });
  });
});
