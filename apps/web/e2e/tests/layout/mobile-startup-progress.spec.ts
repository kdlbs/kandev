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

const BYTE_PROGRESS_BODY = {
  status: "starting",
  startup: {
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
  },
};

const ADVANCED_BYTE_PROGRESS_BODY = {
  ...BYTE_PROGRESS_BODY,
  startup: {
    ...BYTE_PROGRESS_BODY.startup,
    seq: 2,
    elapsed_ms: 60_000,
    step: {
      ...BYTE_PROGRESS_BODY.startup.step,
      elapsed_ms: 60_000,
      done: 15_389_753_344,
      rate_per_second: 268_435_456,
    },
  },
};

/**
 * AC-PLATFORM-STARTUP-PROGRESS-003.14/003.16 on a phone: readable byte sizes in
 * the restart dialog and the real startup page, with no horizontal overflow.
 * Both surfaces advance from the polled snapshot rather than a reload.
 */
test("shows readable byte progress on the phone restart dialog and startup page", async ({
  testPage,
  prCapture,
  backend,
  request,
}) => {
  await stubAgentRuntimeRestart(testPage);
  let readyBody: unknown = BYTE_PROGRESS_BODY;
  await testPage.route("**/ready", (route) =>
    route.fulfill({
      status: 503,
      contentType: "application/json",
      body: JSON.stringify(readyBody),
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
  await expect(dialog).toContainText("Database backup");
  await expect(dialog).toContainText("1.0 MB");
  await expect(dialog.getByRole("progressbar")).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage, "phone restart dialog byte progress");

  readyBody = ADVANCED_BYTE_PROGRESS_BODY;
  await expect(dialog).toContainText("14.3 GB");
  await waitForFiniteAnimations(dialog);
  await prCapture.screenshot("restart-byte-progress-mobile", {
    caption: "Phone restart dialog showing a readable backup size",
  });

  {
    const snapshot = BYTE_PROGRESS_BODY.startup;
    const html = await request
      .post(`${backend.baseUrl}/api/v1/e2e/startup-page-fixture`, { data: snapshot })
      .then((res) => res.text());
    readyBody = BYTE_PROGRESS_BODY;
    await testPage.route("**/startup-byte-fixture", (route) =>
      route.fulfill({ status: 503, contentType: "text/html", body: html }),
    );
    await testPage.goto("/startup-byte-fixture");
    await expect(testPage.locator("#startup-step-label")).toHaveText("Database backup");
    await expect(testPage.locator("#startup-progress")).toHaveText("1.0 MB");
    await expect(testPage.locator("#startup-bar")).toBeHidden();
    await assertNoDocumentHorizontalOverflow(testPage, "phone startup byte progress page");

    readyBody = ADVANCED_BYTE_PROGRESS_BODY;
    await expect(testPage.locator("#startup-progress")).toHaveText("14.3 GB");
    await expect(testPage.locator("#startup-bar")).toBeHidden();
    await prCapture.screenshot("startup-byte-progress-mobile", {
      caption: "Phone startup page showing a readable backup size",
    });
  }
});
