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

test("renders startup progress in the mobile restart dialog", async ({ testPage }) => {
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
  await assertNoDocumentHorizontalOverflow(testPage, "mobile startup progress dialog");
});
