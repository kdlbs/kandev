import type { Page, Route } from "@playwright/test";

type HeldMutation = {
  backendResponseReady: Promise<void>;
  requestCount: () => number;
  release: () => void;
  dispose: () => Promise<void>;
};

/**
 * Lets the backend publish its lifecycle event while the browser holds the
 * matching mutation response. This observes the interval where a removal
 * coordinator must keep outgoing task content out of the DOM.
 */
export async function holdMutationResponse(
  page: Page,
  urlPattern: string,
  method: "DELETE" | "POST",
): Promise<HeldMutation> {
  let resolveReady!: () => void;
  const backendResponseReady = new Promise<void>((resolve) => {
    resolveReady = resolve;
  });
  let resolveRelease!: () => void;
  const releasePromise = new Promise<void>((resolve) => {
    resolveRelease = resolve;
  });
  let released = false;
  let requests = 0;

  const handler = async (route: Route) => {
    if (route.request().method() !== method) {
      await route.continue();
      return;
    }

    requests += 1;
    const response = await route.fetch();
    resolveReady();
    await releasePromise;
    await route.fulfill({ response });
  };

  await page.route(urlPattern, handler);

  return {
    backendResponseReady,
    requestCount: () => requests,
    release: () => {
      if (released) return;
      released = true;
      resolveRelease();
    },
    dispose: () => page.unroute(urlPattern, handler),
  };
}
