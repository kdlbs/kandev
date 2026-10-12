import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import type { AppState } from "@/lib/state/store";
import type { Locator, Page } from "@playwright/test";

type MotionWindow = Window & {
  __KANDEV_E2E_STORE__?: { getState(): AppState };
  __chatMotionSamples?: { text: string; opacity: number; kind: string }[];
  __chatScrollGeometryReads?: number;
};

async function observeMotion(page: Page) {
  await page.evaluate(() => {
    const win = window as MotionWindow;
    win.__chatMotionSamples = [];
    const original = Element.prototype.animate;
    Element.prototype.animate = function (...args: Parameters<Element["animate"]>) {
      const animation = original.apply(this, args);
      if (
        this.hasAttribute("data-chat-motion-item") ||
        this.hasAttribute("data-chat-text-motion")
      ) {
        win.__chatMotionSamples!.push({
          text: this.textContent ?? "",
          opacity: Number(getComputedStyle(this).opacity),
          kind: this.hasAttribute("data-chat-text-motion") ? "text" : "row",
        });
      }
      return animation;
    };
  });
}

async function withScrollHeightObservation(scroller: Locator, run: () => Promise<void>) {
  await scroller.evaluate((element) => {
    let prototype: object | null = Object.getPrototypeOf(element);
    let getter: PropertyDescriptor["get"];
    while (prototype && !getter) {
      getter = Object.getOwnPropertyDescriptor(prototype, "scrollHeight")?.get;
      prototype = Object.getPrototypeOf(prototype);
    }
    if (!getter) throw new Error("Could not observe the transcript scrollHeight getter");
    const readScrollHeight = getter;
    const win = element.ownerDocument.defaultView as MotionWindow;
    win.__chatScrollGeometryReads = 0;
    Object.defineProperty(element, "scrollHeight", {
      configurable: true,
      get: () => {
        win.__chatScrollGeometryReads = (win.__chatScrollGeometryReads ?? 0) + 1;
        return readScrollHeight.call(element);
      },
    });
  });
  try {
    await run();
  } finally {
    await scroller.evaluate((element) => {
      delete (element as unknown as { scrollHeight?: number }).scrollHeight;
      delete (element.ownerDocument.defaultView as MotionWindow).__chatScrollGeometryReads;
    });
  }
}

async function scrollHeightReadCount(page: Page) {
  return page.evaluate(() => (window as MotionWindow).__chatScrollGeometryReads ?? 0);
}

async function expectStableScrollGeometry(scroller: Locator) {
  let previous = "";
  let stableSamples = 0;
  await expect
    .poll(
      async () => {
        const geometry = await scroller.evaluate((el) =>
          [el.scrollHeight, el.clientHeight, el.scrollTop].join(":"),
        );
        stableSamples = geometry === previous ? stableSamples + 1 : 0;
        previous = geometry;
        return stableSamples;
      },
      { intervals: [50], message: "transcript scroll geometry should settle" },
    )
    .toBeGreaterThanOrEqual(3);
}

export function chatMotionScenarios(mobile: boolean) {
  test("chat motion preference previews, resets and persists on this device", async ({
    testPage,
  }) => {
    await testPage.emulateMedia({ reducedMotion: "no-preference" });
    await testPage.goto("/settings/preferences/appearance");
    const card = testPage.getByTestId("chat-motion-settings-card");
    const toggle = card.getByRole("switch", { name: "Chat animations", exact: true });
    await expect(toggle).toBeChecked();
    await toggle.scrollIntoViewIfNeeded();
    if (mobile) {
      const box = await toggle.boundingBox();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
    }
    await toggle.click();
    const save = testPage.getByTestId("settings-floating-save");
    await save.getByRole("button", { name: "Reset", exact: true }).click();
    await expect(toggle).toBeChecked();
    await toggle.click();
    await save.getByRole("button", { name: "Save changes", exact: true }).click();
    await expect(save).not.toBeVisible();
    await testPage.reload();
    await expect(toggle).not.toBeChecked();
    await expect(
      testPage.getByRole("switch", { name: "Animate rich-output charts", exact: true }),
    ).toBeChecked();
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await toggle.scrollIntoViewIfNeeded();
    await testPage.screenshot({
      path: test
        .info()
        .outputPath(mobile ? "chat-motion-settings-phone.png" : "chat-motion-settings-desktop.png"),
    });
  });

  test("live prose and rows animate, history and reduced motion stay static", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await testPage.emulateMedia({ reducedMotion: "no-preference" });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Chat motion",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Missing session");
    await waitForSessionDone(apiClient, task.id, task.session_id, "motion seed should finish");
    // Establish the active turn before opening history. Creating a turn in the
    // observed viewport triggers a history refresh, which intentionally stays static.
    await apiClient.seedAgentMessages(task.session_id, 1, "Existing turn");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    await expect
      .poll(() =>
        testPage.evaluate((id) => {
          const meta = (window as MotionWindow).__KANDEV_E2E_STORE__!.getState().messages
            .metaBySession[id];
          return meta?.historyInitialized && !meta.isLoading;
        }, task.session_id!),
      )
      .toBe(true);
    await observeMotion(testPage);
    await apiClient.seedAgentMessages(task.session_id, 1, "MOTION-PROSE");
    await expect(session.activeChat().getByText("MOTION-PROSE 1", { exact: true })).toBeVisible();
    await expect
      .poll(() =>
        testPage.evaluate(() =>
          (window as MotionWindow).__chatMotionSamples!.some(
            (sample) => sample.kind === "row" && sample.opacity < 1,
          ),
        ),
      )
      .toBe(true);

    // Deliver a deterministic content update through the same action as the WS handler.
    await testPage.evaluate((sessionId) => {
      const state = (window as MotionWindow).__KANDEV_E2E_STORE__!.getState();
      const message = state.messages.bySession[sessionId].find(
        (item) => item.content === "MOTION-PROSE 1",
      )!;
      state.updateMessage({
        ...message,
        content: message.content + " **new suffix** `code`",
        updated_at: new Date().toISOString(),
      });
    }, task.session_id);
    await expect(session.activeChat().getByText("new suffix", { exact: true })).toBeVisible();
    await expect
      .poll(() =>
        testPage.evaluate(() =>
          (window as MotionWindow).__chatMotionSamples!.some(
            (sample) =>
              sample.kind === "text" && sample.text.includes("new suffix") && sample.opacity < 1,
          ),
        ),
      )
      .toBe(true);
    await expect(session.activeChat().locator("code [data-chat-text-motion]")).toHaveCount(0);
    await expect(session.activeChat().locator("[data-chat-text-motion]")).toHaveCount(0);

    await testPage.emulateMedia({ reducedMotion: "reduce" });
    await testPage.evaluate(() => {
      (window as MotionWindow).__chatMotionSamples = [];
    });
    await apiClient.seedAgentMessages(task.session_id, 1, "STATIC-PROSE");
    await expect(session.activeChat().getByText("STATIC-PROSE 1", { exact: true })).toBeVisible();
    // The media-query CSS applies before React handles the change event.
    // Even an effect started during that handoff must stay visually static.
    expect(
      await testPage.evaluate(() =>
        (window as MotionWindow).__chatMotionSamples!.filter((sample) => sample.opacity < 1),
      ),
    ).toEqual([]);
    await expect
      .poll(() =>
        session
          .activeChat()
          .evaluate(
            (el) =>
              el
                .getAnimations({ subtree: true })
                .filter(
                  (animation) =>
                    animation.effect instanceof KeyframeEffect &&
                    (animation.effect.target as Element | null)?.matches(
                      "[data-chat-motion-item], [data-chat-text-motion]",
                    ) &&
                    animation.playState === "running",
                ).length,
          ),
      )
      .toBe(0);
    await testPage.emulateMedia({ reducedMotion: "no-preference" });
    await apiClient.seedAgentMessages(task.session_id, 25, "SCROLL-HISTORY");
    const scroller = session.activeChat().locator(".chat-message-list");
    await expect
      .poll(() => scroller.evaluate((el) => el.scrollHeight - el.clientHeight))
      .toBeGreaterThan(200);
    await expect
      .poll(() => scroller.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop))
      .toBeLessThan(3);
    const lastParagraph = session.activeChat().getByText("SCROLL-HISTORY 25", { exact: true });
    if (mobile) await lastParagraph.tap();
    else await lastParagraph.click();
    const zoomedPosition = await scroller.evaluate((el) => {
      el.style.zoom = "1.125";
      const target = el.scrollHeight - el.clientHeight;
      el.scrollTop = target - 0.25;
      return {
        zoom: getComputedStyle(el).zoom,
        top: el.scrollTop,
        target: el.scrollHeight - el.clientHeight,
      };
    });
    expect(zoomedPosition.zoom).toBe("1.125");
    expect(Number.isInteger(zoomedPosition.top)).toBe(false);
    expect(Math.abs(zoomedPosition.top - zoomedPosition.target)).toBeLessThan(1);
    await withScrollHeightObservation(scroller, async () => {
      const movement = await testPage.evaluate(async (sessionId) => {
        const state = (window as MotionWindow).__KANDEV_E2E_STORE__!.getState();
        const message = state.messages.bySession[sessionId].find(
          (item) => item.content === "STATIC-PROSE 1",
        )!;
        const el = [...document.querySelectorAll<HTMLElement>(".chat-message-list")].find(
          (node) => node.clientHeight > 0,
        )!;
        const start = el.scrollTop;
        state.updateMessage({
          ...message,
          content:
            message.content +
            "\n\n" +
            Array.from({ length: 20 }, (_, i) => `Streaming paragraph ${i}`).join("\n\n"),
          updated_at: new Date().toISOString(),
        });
        const samples: number[] = [];
        for (let i = 0; i < 24; i++) {
          await new Promise(requestAnimationFrame);
          samples.push(el.scrollTop);
        }
        return { start, samples, target: el.scrollHeight - el.clientHeight };
      }, task.session_id);
      expect(
        movement.samples.some((top) => top > movement.start && top < movement.target - 2),
      ).toBe(true);
      expect(Math.abs(movement.samples.at(-1)! - movement.target)).toBeLessThan(3);
      await expect
        .poll(() => scroller.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop))
        .toBeLessThan(2);
      await expectStableScrollGeometry(scroller);
      const readsAfterStreamSettlement = await scrollHeightReadCount(testPage);
      await dwell(
        testPage,
        350,
        "negative-assertion",
        "verify transcript geometry reads stop after stream completion",
      );
      expect(await scrollHeightReadCount(testPage)).toBe(readsAfterStreamSettlement);

      await testPage.evaluate((sessionId) => {
        const state = (window as MotionWindow).__KANDEV_E2E_STORE__!.getState();
        const message = state.messages.bySession[sessionId].find((item) =>
          item.content.startsWith("STATIC-PROSE 1"),
        )!;
        state.updateMessage({
          ...message,
          content:
            message.content +
            "\n\n" +
            Array.from({ length: 10 }, (_, i) => `LATE-GROWTH-${i + 1}`).join("\n\n"),
          updated_at: new Date().toISOString(),
        });
      }, task.session_id);
      await expect(session.activeChat().getByText("LATE-GROWTH-10", { exact: true })).toBeVisible();
      await expect
        .poll(() => scroller.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop))
        .toBeLessThan(2);
      await expectStableScrollGeometry(scroller);
      const readsAfterLaterGrowth = await scrollHeightReadCount(testPage);
      await dwell(
        testPage,
        350,
        "negative-assertion",
        "verify transcript geometry reads stop after later content growth",
      );
      expect(await scrollHeightReadCount(testPage)).toBe(readsAfterLaterGrowth);
    });
    await scroller.evaluate((el) => {
      el.style.zoom = "";
    });
    await expectStableScrollGeometry(scroller);
    await expect
      .poll(() => scroller.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop))
      .toBeLessThan(2);
    if (prCapture.capturing) {
      await prCapture.screenshot(
        mobile ? "chat-scroll-settled-phone" : "chat-scroll-settled-desktop",
        {
          caption: mobile
            ? "Phone transcript settled after later content growth"
            : "Desktop transcript settled after later content growth",
        },
      );
    }
    // Real input must release follow intent before the next delivery.
    if (mobile) {
      const box = (await scroller.boundingBox())!;
      const client = await testPage.context().newCDPSession(testPage);
      const point = { x: box.x + box.width / 2, y: box.y + box.height / 4 };
      await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [point] });
      await client.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ ...point, y: point.y + box.height / 2 }],
      });
      await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
      await client.detach();
    } else {
      await scroller.hover();
      await testPage.mouse.wheel(0, -350);
    }
    await expect
      .poll(() => scroller.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop))
      .toBeGreaterThan(100);
    // Wheel input need not emit scrollend in every browser. Observe a settled
    // position after proving that real input moved away from the bottom.
    let previousTop = -1;
    let stableReads = 0;
    await expect
      .poll(
        async () => {
          const top = await scroller.evaluate((el) => el.scrollTop);
          stableReads = top === previousTop ? stableReads + 1 : 0;
          previousTop = top;
          return stableReads;
        },
        { intervals: [100], message: "reader scroll should settle" },
      )
      .toBeGreaterThanOrEqual(3);
    const heldTop = await scroller.evaluate((el) => el.scrollTop);
    await apiClient.seedAgentMessages(task.session_id, 1, "READER-OWNED");
    await expect(session.activeChat().getByText("READER-OWNED 1", { exact: true })).toBeAttached();
    expect(Math.abs((await scroller.evaluate((el) => el.scrollTop)) - heldTop)).toBeLessThan(3);
    await testPage.reload();
    await session.waitForLoad();
    expect(await session.activeChat().locator("[data-chat-text-motion]").count()).toBe(0);
    await testPage.screenshot({
      path: test.info().outputPath(mobile ? "chat-motion-phone.png" : "chat-motion-desktop.png"),
    });
  });
}
