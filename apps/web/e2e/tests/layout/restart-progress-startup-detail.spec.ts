import { expect, test } from "../../fixtures/test-base";
import {
  setAgentRuntimeAvailability,
  stubAgentRuntimeRestart,
} from "../../helpers/agent-runtime-availability";

type ReadyMode = "progress" | "unreachable" | "ready";

const PROGRESS_BODY = {
  service: "kandev",
  version: "e2e-test",
  status: "starting",
  startup: {
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
      rate_per_second: 33.3,
      eta_ms: 30_000,
      since_advance_ms: 500,
      stalled: false,
    },
  },
};

const UNREACHABLE_BODY = { error: "internal error" };

const READY_BODY = { service: "kandev", version: "e2e-test", status: "ok" };

const STALLED_BODY = {
  service: "kandev",
  version: "e2e-test",
  status: "starting",
  startup: {
    phase: "applying_migrations",
    boot: 1,
    seq: 1,
    elapsed_ms: 130_000,
    phase_elapsed_ms: 130_000,
    step: {
      id: "stores.services",
      label_key: "startup.step.stores_services",
      measure: "counted",
      unit: "messages",
      elapsed_ms: 130_000,
      done: 400,
      total: 1000,
      since_advance_ms: 125_000,
      stalled: true,
    },
  },
};

/**
 * Drives RestartProgressDialog through a real (fully stubbed) restart and
 * asserts the step-level detail added on top of the base phase/elapsed
 * rendering (AC-PLATFORM-STARTUP-PROGRESS-003): step label, counted
 * progress text, ETA, and the AC-PLATFORM-STARTUP-PROGRESS-002.6 status-code
 * fallback rule for a response with no parseable snapshot - a failing status
 * falls back to last-known, a successful one does not.
 */
test.describe("Restart progress dialog startup detail", () => {
  test("renders step label and progress, and resolves a snapshot-free response by status", async ({
    testPage,
  }) => {
    test.setTimeout(60_000);

    let readyMode: ReadyMode = "progress";
    await stubAgentRuntimeRestart(testPage);
    await testPage.route("**/ready", (route) => {
      if (readyMode === "progress") {
        return route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify(PROGRESS_BODY),
        });
      }
      if (readyMode === "unreachable") {
        return route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify(UNREACHABLE_BODY),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(READY_BODY),
      });
    });

    await testPage.goto("/stats");
    await expect(testPage.getByTestId("app-shell")).toBeVisible();

    await setAgentRuntimeAvailability(testPage, {
      status: "unavailable",
      reason: "agentctl_exited",
      occurred_at: "2026-09-18T00:00:00Z",
    });
    const alert = testPage.getByTestId("agent-runtime-alert");
    await expect(alert).toBeVisible();
    await alert.getByRole("button", { name: "Restart Kandev" }).click();

    const dialog = testPage.getByTestId("restart-progress-dialog");
    await expect(dialog).toHaveAttribute("data-phase", "restarting");

    await expect(dialog).toContainText("Applying migrations");
    await expect(dialog).toContainText("Service stores");
    await expect(dialog).toContainText("400 of 1000 messages");
    await expect(dialog).toContainText("About 30 seconds remaining");
    await expect(dialog.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "40");
    await expect(dialog.getByText("Last known")).toHaveCount(0);

    // A failing status with no parseable snapshot falls back to last-known
    // (AC-PLATFORM-STARTUP-PROGRESS-002.6/003.13): the prior step detail
    // stays on screen, labelled stale rather than cleared.
    readyMode = "unreachable";

    await expect(dialog.getByText("Last known")).toBeVisible();
    await expect(dialog).toContainText("Service stores");
    await expect(dialog).toContainText("400 of 1000 messages");

    // A successful status with no parseable snapshot means ready, not stale
    // (AC-PLATFORM-STARTUP-PROGRESS-002.6): the status code is authoritative,
    // so "Last known" must clear even though this response has no snapshot
    // either.
    readyMode = "ready";

    await expect(dialog.getByText("Last known")).toHaveCount(0);
  });

  /**
   * Covers AC-PLATFORM-STARTUP-PROGRESS-004.6: the Settings restart dialog
   * must distinguish a stalled step from a running one, state how long it has
   * gone without progress, and clear that state within one refresh interval
   * once the step advances again.
   */
  test("shows a stalled step and clears it once progress resumes", async ({ testPage }) => {
    test.setTimeout(60_000);

    let stalled = true;
    await stubAgentRuntimeRestart(testPage);
    await testPage.route("**/ready", (route) => {
      const body = stalled ? STALLED_BODY : PROGRESS_BODY;
      return route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify(body),
      });
    });

    await testPage.goto("/stats");
    await expect(testPage.getByTestId("app-shell")).toBeVisible();

    await setAgentRuntimeAvailability(testPage, {
      status: "unavailable",
      reason: "agentctl_exited",
      occurred_at: "2026-09-18T00:00:00Z",
    });
    const alert = testPage.getByTestId("agent-runtime-alert");
    await expect(alert).toBeVisible();
    await alert.getByRole("button", { name: "Restart Kandev" }).click();

    const dialog = testPage.getByTestId("restart-progress-dialog");
    await expect(dialog).toHaveAttribute("data-phase", "restarting");

    await expect(dialog).toContainText("400 of 1000 messages");
    await expect(dialog).toContainText("Stalled for 3 minutes");

    stalled = false;

    await expect(dialog).not.toContainText("Stalled for 3 minutes");
    await expect(dialog).toContainText("400 of 1000 messages");
  });
});
