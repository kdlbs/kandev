import type { Route } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";

type StartupSnapshot = Record<string, unknown>;

const STARTING_SNAPSHOT: StartupSnapshot = {
  phase: "applying_migrations",
  boot: 1,
  seq: 1,
  elapsed_ms: 12_000,
  phase_elapsed_ms: 12_000,
  step: {
    id: "stores.services",
    label_key: "startup.step.stores_services",
    measure: "counted",
    unit: "messages",
    elapsed_ms: 12_000,
    done: 400,
    total: 1000,
    eta_ms: 30_000,
    stalled: false,
  },
};

const ADVANCED_BODY = {
  status: "starting",
  startup: {
    ...STARTING_SNAPSHOT,
    seq: 2,
    step: { ...(STARTING_SNAPSHOT.step as Record<string, unknown>), done: 700, eta_ms: 10_000 },
  },
};

const READY_BODY = {
  status: "ok",
  startup: { phase: "ready", boot: 1, seq: 3, elapsed_ms: 20_000, phase_elapsed_ms: 500 },
};

const POST_RELOAD_HTML =
  '<!DOCTYPE html><html><body><div data-testid="post-reload-marker">app is up</div></body></html>';

/**
 * Drives the Go-rendered startup page (internal/backendapp/startup_page.go)
 * through a real browser instead of httptest string-matching: the initial
 * markup comes from the production renderer via the E2E-only fixture route,
 * and only /ready and the app navigation are mocked, so the embedded poll
 * script runs unmodified.
 */
test.describe("Startup page (real render)", () => {
  test("polls for step progress and reloads once the snapshot reports ready", async ({
    backend,
    browser,
    request,
  }) => {
    const startingHtml = await request
      .post(`${backend.baseUrl}/api/v1/e2e/startup-page-fixture`, { data: STARTING_SNAPSHOT })
      .then((res) => res.text());

    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    const page = await context.newPage();

    let readyBody: unknown = { status: "starting", startup: STARTING_SNAPSHOT };
    let appIsReady = false;

    const fulfillReady = (route: Route) =>
      route.fulfill({
        status: appIsReady ? 200 : 503,
        contentType: "application/json",
        body: JSON.stringify(readyBody),
      });

    const fulfillAppRoute = (route: Route) =>
      appIsReady
        ? route.fulfill({ status: 200, contentType: "text/html", body: POST_RELOAD_HTML })
        : route.fulfill({ status: 503, contentType: "text/html", body: startingHtml });

    await page.route(
      (url) => url.pathname === "/ready",
      (route) => fulfillReady(route),
    );
    await page.route(
      (url) => url.pathname === "/startup-page-e2e-fixture",
      (route) => fulfillAppRoute(route),
    );

    await page.goto("/startup-page-e2e-fixture");

    await expect(page.locator("#startup-phase")).toHaveText("Applying migrations");
    await expect(page.locator("#startup-step-label")).toHaveText("Service stores");
    await expect(page.locator("#startup-progress")).toHaveText("400 of 1000 messages");
    await expect(page.locator("#startup-eta")).toHaveText("About 30 seconds remaining");

    // Poll-driven update: the embedded script re-fetches /ready on its own
    // schedule and re-renders in place, with no test-issued reload.
    readyBody = ADVANCED_BODY;
    await expect(page.locator("#startup-progress")).toHaveText("700 of 1000 messages");
    await expect(page.locator("#startup-eta")).toHaveText("About 10 seconds remaining");

    // Reload-on-ready: once a poll response's snapshot reports phase "ready",
    // the script calls location.reload() itself; the re-navigation this test
    // observes is the real app route being requested a second time.
    appIsReady = true;
    readyBody = READY_BODY;
    await expect(page.getByTestId("post-reload-marker")).toBeVisible();

    await context.close();
  });
});
