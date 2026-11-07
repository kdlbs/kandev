import { expect, type Locator, type Page } from "@playwright/test";

export const STARTUP_TIMINGS = [
  {
    id: "database.backup",
    label_key: "startup.step.database_backup",
    activation_seq: 1,
    phase: "backing_up_database",
    elapsed_ms: 82,
    completed: true,
  },
  {
    id: "startup.queue_reconciliation",
    label_key: "startup.step.queue_reconciliation",
    activation_seq: 2,
    phase: "recovering_sessions",
    elapsed_ms: 1200,
    completed: false,
  },
];

export const LONG_STARTUP_TIMINGS = Array.from({ length: 32 }, (_, index) => ({
  ...STARTUP_TIMINGS[index % 2],
  activation_seq: index + 1,
  elapsed_ms: 82 + index,
}));

// Hold the final progress response until boot-id completion has rendered.
export async function finishStartupTimingRestart(page: Page, dialog: Locator) {
  let release!: () => void;
  const finalReady = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/ready", async (route) => {
    await finalReady;
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        startup: {
          phase: "ready",
          boot: 2,
          seq: 9,
          elapsed_ms: 4321,
          phase_elapsed_ms: 0,
          completed_steps: STARTUP_TIMINGS.map((step) => ({ ...step, completed: true })),
        },
      }),
    });
  });
  await page.route("**/api/v1/system/info", (route) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ boot_id: "e2e-completed-boot" }),
    }),
  );
  try {
    await expect(dialog).toHaveAttribute("data-phase", "done");
  } finally {
    release();
  }
  await expect(dialog.getByRole("progressbar")).toHaveCount(0);
  await expect(dialog).toContainText("Ready");
  await expect(dialog.getByTestId("startup-timings").getByText("Timing incomplete")).toHaveCount(0);
}
