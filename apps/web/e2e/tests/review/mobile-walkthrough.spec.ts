import { exerciseWalkthroughFeedback, checkFeedbackReload } from "./walkthrough-feedback";
import { watchWs } from "../../helpers/causal-waits";
import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import { expectWalkthroughBehindDialog } from "./walkthrough-layering";

async function seedWalkthroughTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario = "walkthrough-basic",
  doneText = "5-step tour",
): Promise<void> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Walkthrough E2E",
    seedData.agentProfileId,
    {
      description: `/e2e:${scenario}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText(doneText, { exact: false })).toBeVisible({
    timeout: 45_000,
  });
  await session.waitForChatIdle();
}

test.describe("Mobile code walkthrough", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("opens the walkthrough as a bottom-sheet panel", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const ws = watchWs(testPage);
    await seedWalkthroughTask(testPage, apiClient, seedData);

    await testPage.getByTestId("walkthrough-launcher").click();
    const card = testPage.getByTestId("walkthrough-floating");
    await expect(card).toBeVisible({ timeout: 30_000 });
    await expect(card).toHaveAttribute("data-mobile-variant", "bottom-sheet");
    await exerciseWalkthroughFeedback(testPage, card, ws);
    await checkFeedbackReload(testPage, card);
    await prCapture.screenshot("mobile-walkthrough-feedback-controls", {
      caption: "Touch-accessible step feedback and shared step picker",
    });

    const box = await card.boundingBox();
    const viewport = testPage.viewportSize();
    if (!box || !viewport) throw new Error("walkthrough geometry unavailable");

    expect(box.x).toBeLessThanOrEqual(12);
    expect(box.width).toBeGreaterThanOrEqual(viewport.width - 24);
    expect(box.y + box.height).toBeGreaterThanOrEqual(viewport.height - 16);

    await expect(testPage.getByTestId("walkthrough-editor-range")).toBeVisible({
      timeout: 15_000,
    });

    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
    await expect(reviewDialog.locator('[data-walkthrough-active="true"]')).toHaveCount(0);
    await expect(reviewDialog.getByText(/^0 of \d+ files reviewed$/)).toBeVisible();
    await expectWalkthroughBehindDialog(testPage, reviewDialog, [
      { locator: card, name: "walkthrough window" },
      {
        locator: testPage.getByTestId("walkthrough-launcher").locator(".."),
        name: "walkthrough launcher",
      },
    ]);
    await prCapture.screenshot("mobile-review-with-walkthrough", {
      caption: "Mobile Review stays isolated from the task walkthrough",
    });
  });

  // @covers AC-TASKS-WALKTHROUGH-TIME-001.1, AC-TASKS-WALKTHROUGH-TIME-001.3
  // @covers AC-TASKS-WALKTHROUGH-TIME-001.4
  test("shows generation time in English and Portuguese within the phone sheet", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const ws = watchWs(testPage);
    await seedWalkthroughTask(testPage, apiClient, seedData);
    const savedWalkthrough = ws.waitForResponse("task.walkthrough.get");
    await testPage.getByTestId("walkthrough-launcher").tap();
    const savedAt = (await savedWalkthrough).payload.updated_at;
    expect(typeof savedAt).toBe("string");
    const card = testPage.getByTestId("walkthrough-floating");
    const generatedAt = card.getByTestId("walkthrough-generated-at");
    for (const locale of ["en", "pt-pt"]) {
      if (locale === "pt-pt") {
        await testPage.evaluate(() => {
          document.cookie = "kandev_locale=pt-pt; path=/; SameSite=Lax";
        });
        await testPage.reload();
        await testPage.getByTestId("walkthrough-launcher").tap();
      }
      await expect(generatedAt).toHaveCount(1);
      await expect(generatedAt).toHaveAttribute("datetime", savedAt as string);
      const formatted = await testPage.evaluate(
        ({ value, locale }) =>
          new Intl.DateTimeFormat(locale, {
            year: "numeric",
            month: "short",
            day: "numeric",
            hour: "numeric",
            minute: "2-digit",
          }).format(new Date(value)),
        { value: savedAt as string, locale },
      );
      await expect(generatedAt).toHaveText(
        `${locale === "en" ? "Generated" : "Gerado em"} ${formatted}`,
      );
      const bounds = await card.boundingBox();
      const timeBounds = await generatedAt.boundingBox();
      const badgeTextLeft = await card.getByTestId("walkthrough-badge").evaluate((badge) => {
        const range = document.createRange();
        range.selectNodeContents(badge);
        return range.getBoundingClientRect().x;
      });
      expect(timeBounds!.x).toBeCloseTo(badgeTextLeft, 0);
      await expect(generatedAt).toHaveCSS("text-align", "left");
      expect(timeBounds!.x).toBeGreaterThanOrEqual(bounds!.x);
      expect(timeBounds!.x + timeBounds!.width).toBeLessThanOrEqual(bounds!.x + bounds!.width);
      expect(timeBounds!.y + timeBounds!.height).toBeLessThanOrEqual(bounds!.y + bounds!.height);
      expect(await generatedAt.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
      await expect(card.getByTestId("walkthrough-close")).toBeVisible();
      await expect(card.getByTestId("walkthrough-next")).toBeVisible();
      expect(
        await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
      ).toBe(true);
      await prCapture.screenshot(`walkthrough-generation-time-${locale}`, {
        caption: `Walkthrough generation time in the phone sheet (${locale})`,
      });
    }
  });

  // @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.4
  test("selects a step with one touch after the viewport height changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);
    await testPage.getByTestId("walkthrough-launcher").tap();
    const card = testPage.getByTestId("walkthrough-floating");
    const picker = card.getByTestId("walkthrough-step-picker");
    const viewport = testPage.viewportSize()!;
    for (const index of [1, 4, 0]) {
      await picker.tap();
      const sheet = testPage.getByRole("dialog", { name: "Go to step" });
      await expect(sheet).toHaveAttribute("data-vaul-drawer-direction", "bottom");
      expect(await sheet.evaluate((element) => getComputedStyle(element).animationName)).not.toBe(
        "none",
      );
      await expect(sheet.locator('[data-slot="command"]')).toHaveCSS(
        "background-color",
        "rgba(0, 0, 0, 0)",
      );
      await expect(testPage.getByRole("option").nth(index)).toBeVisible();
      await testPage.setViewportSize({
        width: viewport.width,
        height: viewport.height - (index ? 64 : 0),
      });
      await expect(testPage.getByRole("option").nth(index)).toBeVisible();
      await testPage.getByRole("option").nth(index).tap();
      await expect(card.getByTestId("walkthrough-step-header")).toContainText(
        `Step ${index + 1} / 5`,
      );
      await expect(picker).toHaveAttribute("aria-expanded", "false");
    }
    await picker.focus();
    await picker.press("ArrowDown");
    await expect(testPage.locator('[data-slot="command"]')).toBeFocused();
    await testPage.keyboard.press("ArrowDown");
    await testPage.keyboard.press("Enter");
    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 2 / 5");
    await expect(picker).toHaveAttribute("aria-expanded", "false");
    await expect(testPage.getByRole("dialog", { name: "Go to step" })).toBeHidden();
    await testPage.emulateMedia({ reducedMotion: "reduce" });
    await picker.tap();
    await expect(testPage.getByRole("dialog", { name: "Go to step" })).toHaveCSS(
      "animation-name",
      "none",
    );
    await expect(testPage.locator('[data-slot="drawer-overlay"]')).toHaveCSS(
      "animation-name",
      "none",
    );
  });

  // @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-003.4
  for (const withoutClicks of [false, true]) {
    test(
      withoutClicks
        ? "selects touch steps without browser click events"
        : "selects steps with consecutive first taps",
      async ({ testPage, apiClient, seedData }) => {
        await seedWalkthroughTask(testPage, apiClient, seedData);
        await testPage.getByTestId("walkthrough-launcher").tap();
        const card = testPage.getByTestId("walkthrough-floating");
        const picker = card.getByTestId("walkthrough-step-picker");
        if (withoutClicks) {
          // Reproduce the physical iPhone trace: completed touches without a click.
          await testPage.evaluate(() => {
            document.addEventListener(
              "click",
              (event) => {
                if (event.detail === 0) return;
                event.preventDefault();
                event.stopImmediatePropagation();
              },
              true,
            );
          });
        }
        for (const index of [4, 0, 3, 1, 2, 0]) {
          const triggerBox = (await picker.boundingBox())!;
          // Wait for sheet motion explicitly; send each touch only once.
          await testPage.touchscreen.tap(
            triggerBox.x + triggerBox.width / 2,
            triggerBox.y + triggerBox.height / 2,
          );
          const option = testPage.getByRole("option").nth(index);
          await expect(option).toBeVisible();
          const sheet = testPage.getByRole("dialog", { name: "Go to step" });
          await sheet.evaluate(async (element) => {
            await Promise.all(element.getAnimations().map((animation) => animation.finished));
          });
          const optionBox = (await option.boundingBox())!;
          await testPage.touchscreen.tap(
            optionBox.x + optionBox.width / 2,
            optionBox.y + optionBox.height / 2,
          );
          await expect(card.getByTestId("walkthrough-step-header")).toContainText(
            `Step ${index + 1} / 5`,
          );
          await expect(picker).toHaveAttribute("aria-expanded", "false");
          await expect(sheet).toBeHidden();
        }
      },
    );
  }

  // @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.4
  test("keeps composer navigation reachable on landscape phones", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);
    await testPage.getByTestId("walkthrough-launcher").tap();
    const card = testPage.getByTestId("walkthrough-floating");
    await card.getByTestId("walkthrough-comment-trigger").tap();
    await card.getByRole("textbox").fill("Keep this landscape draft.\n".repeat(20));
    const next = card.getByTestId("walkthrough-next");
    for (const { width, height } of [
      { width: 639, height: 375 },
      { width: 640, height: 375 },
      { width: 667, height: 375 },
      { width: 767, height: 375 },
      { width: 667, height: 320 },
    ]) {
      await testPage.setViewportSize({ width, height });
      await expect
        .poll(() =>
          next.evaluate((button) => {
            const rect = button.getBoundingClientRect();
            const card = button
              .closest('[data-testid="walkthrough-floating"]')!
              .getBoundingClientRect();
            return (
              rect.height >= 44 &&
              rect.top >= card.top &&
              rect.bottom <= Math.min(card.bottom, innerHeight) &&
              button.contains(
                document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
              )
            );
          }),
        )
        .toBe(true);
      await expect
        .poll(async () => {
          const actions = (await card.locator("[data-comment-actions]").boundingBox())!;
          const navigation = (await card.getByTestId("walkthrough-navigation").boundingBox())!;
          return actions.y + actions.height <= navigation.y;
        })
        .toBe(true);
      await expect
        .poll(() =>
          card.getByRole("textbox").evaluate((input) => {
            const actions = input.parentElement!.querySelector("[data-comment-actions]")!;
            const bounds = input.getBoundingClientRect();
            return bounds.height >= 44 && bounds.bottom <= actions.getBoundingClientRect().top;
          }),
        )
        .toBe(true);
    }
    await next.tap();
    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 2 / 5");
    await card.getByTestId("walkthrough-prev").tap();
    await card.getByTestId("walkthrough-comment-trigger").tap();
    await expect(card.getByRole("textbox")).toHaveValue("Keep this landscape draft.\n".repeat(20));
  });

  // @covers AC-UI-WALKTHROUGH-STEP-FEEDBACK-002.4
  for (const withoutClicks of [false, true]) {
    test(`navigates Previous and Next with single taps${withoutClicks ? " without click events" : ""}`, async ({
      testPage,
      apiClient,
      seedData,
    }) => {
      await seedWalkthroughTask(testPage, apiClient, seedData);
      await testPage.getByTestId("walkthrough-launcher").tap();
      const card = testPage.getByTestId("walkthrough-floating");
      const previous = card.getByTestId("walkthrough-prev");
      const next = card.getByTestId("walkthrough-next");
      if (withoutClicks) {
        await testPage.evaluate(() => {
          document.addEventListener(
            "click",
            (event) => {
              if (event.detail === 0) return;
              event.preventDefault();
              event.stopImmediatePropagation();
            },
            true,
          );
        });
      }
      await expect(previous).toBeDisabled();
      for (const { button, steps } of [
        { button: next, steps: [2, 3, 4, 5] },
        { button: previous, steps: [4, 3, 2, 1] },
      ]) {
        for (const step of steps) {
          const box = (await button.boundingBox())!;
          expect(box.height).toBeGreaterThanOrEqual(44);
          await testPage.touchscreen.tap(box.x + box.width / 2, box.y + box.height / 2);
          await expect(card.getByTestId("walkthrough-step-header")).toContainText(
            `Step ${step} / 5`,
          );
        }
        await expect(button).toBeDisabled();
        const boundaryBox = (await button.boundingBox())!;
        const boundaryPointer = {
          pointerType: "touch",
          pointerId: 1,
          isPrimary: true,
          button: 0,
          clientX: boundaryBox.x + boundaryBox.width / 2,
          clientY: boundaryBox.y + boundaryBox.height / 2,
        };
        await button.dispatchEvent("pointerdown", boundaryPointer);
        await button.dispatchEvent("pointerup", boundaryPointer);
        await expect(card.getByTestId("walkthrough-step-header")).toContainText(
          `Step ${steps.at(-1)} / 5`,
        );
      }
      await expect(previous).toBeDisabled();
      await next.focus();
      await testPage.keyboard.press("Enter");
      await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 2 / 5");
      await previous.focus();
      await testPage.keyboard.press("Space");
      await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 1 / 5");
    });
  }

  test("requests a walkthrough from Changes and opens the generated bottom sheet", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-setup", "changes ready");

    await testPage
      .getByRole("navigation")
      .getByRole("button", { name: /Changes$/ })
      .click();
    const changes = testPage.getByTestId("mobile-changes-panel");
    await expect(changes).toBeVisible({ timeout: 15_000 });
    const request = changes.getByTestId("changes-request-walkthrough");
    await expect(request).toBeEnabled({ timeout: 30_000 });
    await expect(request.getByText("Walkthrough", { exact: true })).toBeVisible();
    const [changesBox, requestBox] = await Promise.all([
      changes.boundingBox(),
      request.boundingBox(),
    ]);
    if (!changesBox || !requestBox) throw new Error("Mobile Changes toolbar geometry unavailable");
    expect(requestBox.x).toBeGreaterThanOrEqual(changesBox.x);
    expect(requestBox.x + requestBox.width).toBeLessThanOrEqual(
      changesBox.x + changesBox.width + 1,
    );
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await prCapture.screenshot("mobile-changes-walkthrough", {
      caption: "Mobile Changes keeps the Walkthrough action within the viewport",
    });
    await request.click();

    await testPage.getByRole("button", { name: "Chat", exact: true }).click();
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.activeChat()).toContainText("Walkthrough: Tour of the change", {
      timeout: 45_000,
    });
    await expect(session.activeChat()).toContainText("walkthrough-request complete", {
      timeout: 45_000,
    });

    await session.walkthroughLauncher().click();
    const card = session.walkthroughFloating();
    await expect(card).toBeVisible({ timeout: 30_000 });
    await expect(card).toHaveAttribute("data-mobile-variant", "bottom-sheet");
    await expect(card.getByTestId("walkthrough-step-title")).toContainText("Overview");
    await expect(card.getByTestId("walkthrough-step-body")).toContainText("ELI5:");
    await expect(session.walkthroughEditorRange()).toBeVisible({ timeout: 15_000 });

    const initialNavigation = await card.getByTestId("walkthrough-navigation").boundingBox();
    if (!initialNavigation) throw new Error("walkthrough navigation missing before step change");
    await card.getByTestId("walkthrough-next").tap();
    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 2 / 5");
    const nextNavigation = await card.getByTestId("walkthrough-navigation").boundingBox();
    if (!nextNavigation) throw new Error("walkthrough navigation missing after step change");
    expect(Math.abs(nextNavigation.y - initialNavigation.y)).toBeLessThan(1);
    const viewport = testPage.viewportSize();
    if (!viewport) throw new Error("walkthrough viewport unavailable");
    expect(nextNavigation.y).toBeGreaterThanOrEqual(0);
    expect(nextNavigation.y + nextNavigation.height).toBeLessThanOrEqual(viewport.height);
  });

  test("can discard a walkthrough from the touch launcher", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData);
    const session = new SessionPage(testPage);

    await expect(session.walkthroughLauncher()).toBeVisible({ timeout: 30_000 });
    await expect(session.walkthroughDiscardButton()).toBeVisible({ timeout: 5_000 });
    await session.walkthroughDiscardButton().tap();
    const discardConfirmation = session.walkthroughDiscardConfirmation();
    await expect(discardConfirmation).toBeVisible();
    await expect(discardConfirmation).toHaveAttribute("role", "group");
    await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
    await expect(testPage.getByRole("dialog")).toHaveAttribute("data-slot", "drawer-content");
    await prCapture.screenshot("mobile-walkthrough-discard-confirmation", {
      caption: "Mobile walkthrough discard uses a named bottom sheet with stacked actions",
    });
    const actionBoxes = await Promise.all(
      ["Cancel", "Discard walkthrough"].map((name) =>
        discardConfirmation.getByRole("button", { name }).boundingBox(),
      ),
    );
    for (const box of actionBoxes) {
      if (!box) throw new Error("walkthrough discard action geometry unavailable");
      expect(box.width).toBeGreaterThanOrEqual(44);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }
    await discardConfirmation.getByRole("button", { name: "Cancel" }).tap();
    await expect(discardConfirmation).toHaveCount(0);
    await expect(session.walkthroughLauncher()).toBeVisible();

    await session.walkthroughDiscardButton().tap();
    await expect(session.walkthroughDiscardConfirmation()).toBeVisible();
    await session
      .walkthroughDiscardConfirmation()
      .getByRole("button", { name: "Discard walkthrough" })
      .tap();

    await expect(session.walkthroughLauncher()).toHaveCount(0);
    await expect(session.walkthroughFloating()).toHaveCount(0);
  });
});
