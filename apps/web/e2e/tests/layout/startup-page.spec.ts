import { STARTUP_TIMINGS } from "../../helpers/startup-timing-fixture";
import type { Route } from "@playwright/test";
import { expect, test } from "../../fixtures/test-base";

type StartupSnapshot = Record<string, unknown>;

const STARTING_SNAPSHOT: StartupSnapshot = {
  phase: "applying_migrations",
  boot: 1,
  seq: 1,
  elapsed_ms: 12_000,
  phase_elapsed_ms: 12_000,
  completed_steps: STARTUP_TIMINGS,
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
    prCapture,
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

    await expect(page.locator("#startup-timings")).toContainText("82 milliseconds");
    await expect(page.locator("#startup-timings")).toContainText("Timing incomplete");
    await prCapture.screenshot("startup-timings-desktop", {
      page,
      caption: "Startup page with current housekeeping and completed timing history",
    });

    // A failing response with a malformed snapshot must keep the last valid
    // detail and mark it as stale. The status code remains authoritative when
    // the body shape cannot be trusted.
    readyBody = { status: "starting", startup: { ...STARTING_SNAPSHOT, step: {} } };
    await expect(page.getByText("Last known")).toBeVisible();
    await expect(page.locator("#startup-step-label")).toHaveText("Service stores");
    await expect(page.locator("#startup-progress")).toHaveText("400 of 1000 messages");

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

  /**
   * AC-PLATFORM-STARTUP-PROGRESS-003.14/003.16: the same readable size must
   * come from the Go initial render and from the embedded poll script. The
   * first poll is held until the initial HTML has been asserted, so a page
   * that formatted only one of the two would fail rather than pass on the
   * later value.
   */
  test("renders readable byte progress in the initial page and poll updates", async ({
    backend,
    browser,
    request,
    prCapture,
  }) => {
    const byteSnapshot: StartupSnapshot = {
      phase: "backing_up_database",
      boot: 1,
      seq: 1,
      elapsed_ms: 30_000,
      phase_elapsed_ms: 30_000,
      step: {
        id: "database.backup",
        label_key: "startup.step.database_backup",
        measure: "counting",
        unit: "bytes",
        elapsed_ms: 30_000,
        done: 1_048_576,
        rate_per_second: 1_048_576,
        stalled: false,
      },
    };
    const startingHtml = await request
      .post(`${backend.baseUrl}/api/v1/e2e/startup-page-fixture`, { data: byteSnapshot })
      .then((res) => res.text());

    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    const page = await context.newPage();

    let releaseFirstPoll!: () => void;
    const initialRenderAsserted = new Promise<void>((resolve) => {
      releaseFirstPoll = resolve;
    });
    const advancedBody = {
      status: "starting",
      startup: {
        ...byteSnapshot,
        seq: 2,
        elapsed_ms: 60_000,
        step: {
          ...(byteSnapshot.step as Record<string, unknown>),
          elapsed_ms: 60_000,
          done: 15_389_753_344,
          rate_per_second: 268_435_456,
        },
      },
    };
    // A large but exact count whose value sits just below a decimal midpoint.
    // Rounding by hand after a float multiply would render 4097.0 TB here,
    // while the Go renderer and the web helper both render 4096.9 TB.
    const largeCountingBody = {
      status: "starting",
      startup: {
        ...advancedBody.startup,
        seq: 3,
        elapsed_ms: 90_000,
        step: { ...advancedBody.startup.step, elapsed_ms: 90_000, done: 4_504_644_163_416_883 },
      },
    };
    let polledBody: unknown = advancedBody;

    await page.route(
      (url) => url.pathname === "/ready",
      async (route) => {
        await initialRenderAsserted;
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify(polledBody),
        });
      },
    );
    await page.route(
      (url) => url.pathname === "/startup-page-e2e-fixture",
      (route) => route.fulfill({ status: 503, contentType: "text/html", body: startingHtml }),
    );

    try {
      await page.goto("/startup-page-e2e-fixture");
      await expect(page.locator("#startup-step-label")).toHaveText("Database backup");
      await expect(page.locator("#startup-progress")).toHaveText("1.0 MB");
      await expect(page.locator("#startup-progress")).toBeInViewport();
      await expect(page.locator("#startup-bar")).toBeHidden();
    } finally {
      releaseFirstPoll();
    }

    // The embedded poll script re-renders the same policy without a reload.
    await expect(page.locator("#startup-progress")).toHaveText("14.3 GB");
    await expect(page.locator("#startup-progress")).toBeInViewport();
    await expect(page.locator("#startup-bar")).toBeHidden();
    await prCapture.screenshot("startup-byte-progress-desktop", {
      page,
      caption: "Desktop startup page showing a readable backup size",
    });

    polledBody = largeCountingBody;
    await expect(page.locator("#startup-progress")).toHaveText("4096.9 TB");
    await expect(page.locator("#startup-bar")).toBeHidden();

    await context.close();
  });
});
