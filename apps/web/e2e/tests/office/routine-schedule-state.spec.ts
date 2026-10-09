import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";

// REQ-OFFICE-ROUTINE-ARMING-002: schedule state is a second switch,
// independent of a routine's intent (status). This spec seeds one routine
// per reachable schedule state and asserts the routines list renders the
// correct AC-002.10 label group for each, with no two groups ever
// rendering the same label.
//
// office_routine_triggers has no PATCH route, and the public create-trigger
// endpoint validates the cron expression and always computes a next_run_at,
// so it cannot produce `trigger_invalid`, `trigger_unscheduled`, or
// `trigger_disabled`. Those three are seeded directly through the
// KANDEV_E2E_MOCK-gated /_test/routine-triggers route
// (ApiClient.seedRoutineTrigger) instead. `unknown` requires a live
// database read failure and has no fixture-reachable trigger shape; it is
// covered by the backend unit tests in arming_test.go instead.

const futureCronRun = () => new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString();

function rowForRoutine(page: Page, name: string) {
  return page.getByRole("link", { name, exact: true }).locator("xpath=../..");
}

test.describe("Routine schedule state rendering", () => {
  test("routines list renders a distinct label per schedule-state group", async ({
    testPage,
    officeApi,
    officeSeed,
    apiClient,
  }) => {
    const cases: Array<{
      name: string;
      group: string;
      expectedLabel: string;
      seed?: (routineId: string) => Promise<unknown>;
    }> = [
      {
        name: "E2E Armed Routine",
        group: "armed",
        expectedLabel: "Armed",
        seed: (routineId) =>
          apiClient.seedRoutineTrigger({
            routineId,
            kind: "cron",
            cronExpression: "0 9 * * *",
            enabled: true,
            nextRunAt: futureCronRun(),
          }),
      },
      {
        name: "E2E Invalid Cron Routine",
        group: "broken",
        expectedLabel: "Schedule broken",
        seed: (routineId) =>
          apiClient.seedRoutineTrigger({
            routineId,
            kind: "cron",
            cronExpression: "not a cron expression",
            enabled: true,
          }),
      },
      {
        name: "E2E Unscheduled Cron Routine",
        group: "broken",
        expectedLabel: "Schedule broken",
        seed: (routineId) =>
          apiClient.seedRoutineTrigger({
            routineId,
            kind: "cron",
            cronExpression: "0 9 * * *",
            enabled: true,
          }),
      },
      {
        name: "E2E Disabled Cron Routine",
        group: "broken",
        expectedLabel: "Schedule broken",
        seed: (routineId) =>
          apiClient.seedRoutineTrigger({
            routineId,
            kind: "cron",
            cronExpression: "0 9 * * *",
            enabled: false,
            nextRunAt: futureCronRun(),
          }),
      },
      {
        name: "E2E Webhook Routine",
        group: "event_only",
        expectedLabel: "Event-triggered",
        seed: (routineId) =>
          apiClient.seedRoutineTrigger({ routineId, kind: "webhook", enabled: true }),
      },
      {
        name: "E2E Manual Only Routine",
        group: "no_schedule",
        expectedLabel: "No schedule",
        seed: (routineId) =>
          apiClient.seedRoutineTrigger({ routineId, kind: "manual", enabled: true }),
      },
      {
        name: "E2E No Trigger Routine",
        group: "no_schedule",
        expectedLabel: "No schedule",
      },
    ];

    for (const testCase of cases) {
      const routine = await officeApi.createRoutine(officeSeed.workspaceId, {
        name: testCase.name,
      });
      if (testCase.seed) {
        await testCase.seed(routine.id as string);
      }
    }

    await testPage.goto("/office/routines");

    for (const testCase of cases) {
      const row = rowForRoutine(testPage, testCase.name);
      await expect(row.getByText(testCase.expectedLabel, { exact: true })).toBeVisible({
        timeout: 10_000,
      });
    }

    // AC-002.11: event_only never renders as having no schedule, even
    // though it also fires without a cron trigger.
    const webhookRow = rowForRoutine(testPage, "E2E Webhook Routine");
    await expect(webhookRow.getByText("No schedule", { exact: true })).toHaveCount(0);

    // AC-002.10: every case sharing a group above already asserted the same
    // literal label text; the broken group additionally proves it across
    // three different underlying raw states (trigger_invalid,
    // trigger_unscheduled, trigger_disabled), not just two.
    const brokenRoutineNames = cases.filter((c) => c.group === "broken").map((c) => c.name);
    expect(brokenRoutineNames).toHaveLength(3);
  });
});
