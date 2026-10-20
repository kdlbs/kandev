import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import {
  finishStartupTimingRestart,
  LONG_STARTUP_TIMINGS,
} from "../../helpers/startup-timing-fixture";
import { expect, test } from "../../fixtures/test-base";
import {
  setAgentRuntimeAvailability,
  stubAgentRuntimeRestart,
} from "../../helpers/agent-runtime-availability";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const PROGRESS_BODY = {
  status: "starting",
  startup: {
    phase: "applying_migrations",
    boot: 1,
    seq: 1,
    elapsed_ms: 12_000,
    phase_elapsed_ms: 12_000,
    completed_steps: LONG_STARTUP_TIMINGS,
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
  },
};

test("renders contained timings in the phone restart dialog and startup page", async ({
  testPage,
  prCapture,
  backend,
  request,
}) => {
  await stubAgentRuntimeRestart(testPage);
  await testPage.route("**/ready", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify(PROGRESS_BODY),
    }),
  );

  await testPage.goto("/stats");
  await expect(testPage.getByTestId("app-shell")).toBeVisible();

  await setAgentRuntimeAvailability(testPage, {
    status: "unavailable",
    reason: "agentctl_exited",
    occurred_at: "2026-09-18T00:00:00Z",
  });
  const alert = testPage.getByTestId("agent-runtime-alert");
  await expect(alert).toBeVisible();
  await alert.getByRole("button", { name: "Restart Kandev" }).tap();

  const dialog = testPage.getByTestId("restart-progress-dialog");
  await expect(dialog).toHaveAttribute("data-phase", "restarting");
  await expect(dialog).toContainText("Service stores");
  await expect(dialog).toContainText("400 of 1000 messages");
  await expect(dialog.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "40");
  const timings = dialog.getByTestId("startup-timings");
  await expect(timings).toContainText("82 milliseconds");
  await expect(timings).toContainText("Timing incomplete");
  expect(await timings.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
  const bounds = await dialog.boundingBox();
  const viewport = testPage.viewportSize()!;
  expect(bounds!.x).toBeGreaterThanOrEqual(0);
  expect(bounds!.y).toBeGreaterThanOrEqual(0);
  expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile startup progress dialog");
  await timings.evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await expect(timings.getByText("113 milliseconds", { exact: true })).toBeVisible();
  await finishStartupTimingRestart(testPage, dialog);
  const reload = dialog.getByRole("button", { name: "Reload page" });
  expect((await reload.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await reload.click({ trial: true });
  await assertNoDocumentHorizontalOverflow(testPage, "completed phone restart dialog");
  await waitForFiniteAnimations(dialog);
  await prCapture.screenshot("restart-timings-mobile", {
    caption: "Completed phone restart dialog with final startup timings",
  });
  {
    const snapshot = PROGRESS_BODY.startup;
    const html = await request
      .post(`${backend.baseUrl}/api/v1/e2e/startup-page-fixture`, { data: snapshot })
      .then((res) => res.text());
    await testPage.route("**/ready", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify(PROGRESS_BODY),
      }),
    );
    await testPage.route("**/startup-timing-fixture", (route) =>
      route.fulfill({ status: 503, contentType: "text/html", body: html }),
    );
    await testPage.goto("/startup-timing-fixture");
    const timings = testPage.locator("#startup-timings");
    await expect(timings).toContainText("82 milliseconds");
    expect(await timings.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "phone startup timing page");
    await prCapture.screenshot("startup-timings-mobile", {
      caption: "Phone startup page with bounded timing history",
    });
    await timings.focus();
    await testPage.keyboard.press("End");
    await expect(timings.getByText("113 milliseconds", { exact: true })).toBeVisible();
    await expect(testPage.locator("#startup-step-label")).toBeInViewport();
  }
});
