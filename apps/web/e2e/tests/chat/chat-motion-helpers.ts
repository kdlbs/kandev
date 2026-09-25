import { test, expect } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import type { AppState } from "@/lib/state/store";
import type { Page } from "@playwright/test";

type MotionWindow = Window & {
  __KANDEV_E2E_STORE__?: { getState(): AppState };
  __chatMotionSamples?: { text: string; opacity: number; kind: string }[];
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
    expect(movement.samples.some((top) => top > movement.start && top < movement.target - 2)).toBe(
      true,
    );
    expect(Math.abs(movement.samples.at(-1)! - movement.target)).toBeLessThan(3);
    // Real input must release follow intent before the next delivery.
    const scrollEnded = scroller.evaluate(
      (el) =>
        new Promise<void>((resolve) => {
          el.addEventListener("scrollend", () => resolve(), { once: true });
        }),
    );
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
    await scrollEnded;
    await expect
      .poll(() => scroller.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop))
      .toBeGreaterThan(100);
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
