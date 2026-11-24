import { expect } from "@playwright/test";
import type { Page, Route } from "@playwright/test";

const GOAL_PATH = /\/coordinators\/[^/]+\/goal(?:\/.*)?$/;

type GoalInputProbe = {
  name: string | null;
  setEnabled: boolean | null;
  afterPaint: boolean;
};

declare global {
  interface Window {
    __goalAdoptionProbe?: GoalInputProbe;
    __goalAdoptionObserver?: MutationObserver;
  }
}

export async function enterGoalNameDuringInitialLoad(
  page: Page,
  settingsUrl: string,
  name: string,
): Promise<GoalInputProbe> {
  let releaseRead: (() => void) | undefined;
  let readIntercepted: (() => void) | undefined;
  const readGate = new Promise<void>((resolve) => {
    releaseRead = resolve;
  });
  const firstReadIntercepted = new Promise<void>((resolve) => {
    readIntercepted = resolve;
  });
  let heldFirstRead = false;

  const routeHandler = async (route: Route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (!heldFirstRead && request.method() === "GET" && GOAL_PATH.test(path)) {
      heldFirstRead = true;
      readIntercepted?.();
      await readGate;
    }
    await route.continue();
  };

  await page.route(GOAL_PATH, routeHandler);
  try {
    await page.goto(settingsUrl);
    await expect(page.getByTestId("goal-loading")).toBeVisible();
    await firstReadIntercepted;
    await page.evaluate((goalName) => {
      const probe: GoalInputProbe = { name: null, setEnabled: null, afterPaint: false };
      const inputSelector = '[data-testid="goal-name"]';
      const submitSelector = '[data-testid="goal-set"]';
      const observer = new MutationObserver(() => {
        const input = document.querySelector<HTMLInputElement>(inputSelector);
        if (!input || input.disabled) return;

        observer.disconnect();
        const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
        if (!setter) throw new Error("Native input value setter is unavailable");
        setter.call(input, goalName);
        input.dispatchEvent(new Event("input", { bubbles: true }));
        requestAnimationFrame(() => {
          requestAnimationFrame(() => {
            const submit = document.querySelector<HTMLButtonElement>(submitSelector);
            probe.name = input.value;
            probe.setEnabled = submit !== null && !submit.disabled;
            probe.afterPaint = true;
          });
        });
      });
      window.__goalAdoptionProbe = probe;
      window.__goalAdoptionObserver = observer;
      observer.observe(document.body, { childList: true, subtree: true, attributes: true });
    }, name);
    releaseRead?.();
    await page.waitForFunction(() => window.__goalAdoptionProbe?.afterPaint === true);
    return await page.evaluate(() => {
      if (!window.__goalAdoptionProbe) throw new Error("Goal input probe did not run");
      return window.__goalAdoptionProbe;
    });
  } finally {
    releaseRead?.();
    await page.unroute(GOAL_PATH, routeHandler);
    await page.evaluate(() => window.__goalAdoptionObserver?.disconnect()).catch(() => undefined);
  }
}
