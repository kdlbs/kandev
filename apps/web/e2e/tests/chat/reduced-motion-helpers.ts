import { expect, type Locator, type Page } from "@playwright/test";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { expectReducedStatusMotion } from "../../helpers/animation-assertions";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionDone } from "../../helpers/session";
import { waitForActiveSessionForegroundActivity } from "../../helpers/session-store";

async function changeMotionPreference(page: Page, reducedMotion: "reduce" | "no-preference") {
  await page.evaluate(() => {
    document.documentElement.dataset.motionPreferenceUpdated = "pending";
    matchMedia("(prefers-reduced-motion: reduce)").addEventListener(
      "change",
      () => {
        document.documentElement.dataset.motionPreferenceUpdated = "done";
      },
      { once: true },
    );
  });
  await page.emulateMedia({ reducedMotion });
  await expect(page.locator("html")).toHaveAttribute("data-motion-preference-updated", "done");
  await page.evaluate(() => {
    delete document.documentElement.dataset.motionPreferenceUpdated;
  });
}

async function verifyHiddenFallbackPreference(page: Page, cubes: Locator) {
  const grid = cubes.first().locator("..");
  await grid.evaluate((element) => {
    (element as HTMLElement).style.transform = "translateX(-200vw)";
  });
  const expectPaused = () =>
    expect
      .poll(() =>
        cubes.evaluateAll((elements) =>
          elements.every((element) => getComputedStyle(element).animationPlayState === "paused"),
        ),
      )
      .toBe(true);
  await expectPaused();
  await changeMotionPreference(page, "reduce");
  await changeMotionPreference(page, "no-preference");
  await expectPaused();
  await grid.evaluate((element) => {
    (element as HTMLElement).style.removeProperty("transform");
  });
}

export async function verifyReducedMotionFallback(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  capture: PrAssetCapture,
) {
  await page.addInitScript(() => {
    Object.defineProperty(Element.prototype, "animate", { configurable: true, value: undefined });
  });
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Reduced motion fallback",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Expected a session for the motion check");
  await waitForSessionDone(
    api,
    task.id,
    task.session_id,
    "initial motion test turn settled",
    30_000,
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await page.evaluate(() => {
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await session.sendMessageViaButton("/slow 15s");
  await waitForActiveSessionForegroundActivity(page, "generating", task.session_id);
  const cubes = session.activeChat().locator(".spinner-grid-cube:visible");
  await expect.poll(() => cubes.count()).toBeGreaterThan(0);
  await expect
    .poll(() =>
      cubes.evaluateAll((elements) =>
        elements.every((element) => getComputedStyle(element).animationPlayState === "paused"),
      ),
    )
    .toBe(true);
  await page.evaluate(() => {
    Reflect.deleteProperty(document, "visibilityState");
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await expect
    .poll(() =>
      cubes.evaluateAll((elements) =>
        elements.every((element) =>
          element
            .getAnimations()
            .some(
              (animation) =>
                animation.constructor.name === "CSSAnimation" && animation.playState === "running",
            ),
        ),
      ),
    )
    .toBe(true);
  await expectReducedStatusMotion(page);
  await verifyHiddenFallbackPreference(page, cubes);
  await expect
    .poll(() =>
      cubes.evaluateAll((elements) =>
        elements.every((element) =>
          element.getAnimations().some((animation) => animation.playState === "running"),
        ),
      ),
    )
    .toBe(true);
  await page.emulateMedia({ reducedMotion: "reduce" });
  const redaction = capture.capturing
    ? await page.addStyleTag({ content: ".xterm { visibility: hidden !important; }" })
    : null;
  try {
    await capture.screenshot("composer-reduced-motion", {
      caption:
        "The busy task composer retains visible grid status under reduced motion with CSS fallback.",
    });
  } finally {
    await redaction?.evaluate((element) => element.remove());
  }
  await waitForSessionDone(api, task.id, task.session_id, "slow motion test turn settled", 30_000);
  await expect(session.activeChat().getByTestId("chat-input-glow")).toHaveCount(0);
}
