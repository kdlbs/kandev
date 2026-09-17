import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";

/**
 * Wire contract round trips (docs/specs/office/requirements/routine-wire-contract.md).
 * The web client and the Go DTOs disagreed on key case for every multi-word
 * routine/trigger field; these specs drive the real Create/Save UI against the
 * running backend and read the raw (snake_case) wire response back, which is
 * the honest round trip `routine-catch-up-policy-ui.spec.ts` could not do
 * before this capability landed.
 */

// Field wrapper in routine-detail-view.tsx renders <Label> then the control as
// a sibling, not a `for`/`id` pair, so `getByLabel` cannot resolve it there
// (the create dialog's steps 0-1 controls do have real `id`s and use
// `getByLabel` directly). This mirrors routine-catch-up-policy-ui.spec.ts's
// own `catchUpPolicyCombobox` helper, generalized to any Field-wrapped control.
function comboboxNear(page: Page, label: string) {
  return page.getByText(label, { exact: true }).locator("..").getByRole("combobox");
}

function textboxNear(page: Page, label: string) {
  return page.getByText(label, { exact: true }).locator("..").getByRole("textbox");
}

test.describe("Routines UI", () => {
  test("routine created via API appears in page", async ({ testPage, officeApi, officeSeed }) => {
    await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Test Routine",
    });
    await testPage.goto("/office/routines");
    await expect(testPage.getByText("E2E Test Routine")).toBeVisible({ timeout: 10_000 });
  });

  // A paused routine's cron-suppression behavior is covered by Go unit
  // tests; this spec covers only the UI surfaces the frontend owns: the
  // row reflects the paused status, and "Run Now" from either surface is
  // refused with the status-gated toast.
  test("paused routine shows off and refuses run now", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Paused Routine",
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    await testPage.goto("/office/routines");
    const row = testPage.getByTestId(`routine-row-${routine.id}`);
    await expect(row).toBeVisible({ timeout: 10_000 });
    await expect(row.getByText("On", { exact: true })).toBeVisible({ timeout: 10_000 });

    const paused = await apiClient.rawRequest("PATCH", `/api/v1/office/routines/${routine.id}`, {
      status: "paused",
    });
    expect(paused.ok).toBe(true);

    await testPage.reload();
    await expect(row).toBeVisible({ timeout: 10_000 });
    await expect(row.getByText("Off", { exact: true })).toBeVisible({ timeout: 10_000 });

    // DropdownMenuContent renders in a portal outside the row's DOM
    // subtree, so the trigger is found scoped to the row but the menu
    // item itself is located page-wide once open.
    const rowRunRefused = waitForHttp(testPage, "POST", /\/routines\/[^/]+\/run$/, {
      predicate: (r) => r.status() === 409,
    });
    await row.getByRole("button").last().click();
    await testPage.getByTestId("routine-run-now").click();
    await rowRunRefused;
    await expect(testPage.getByText(/Cannot run: routine status is paused/i).first()).toBeVisible({
      timeout: 10_000,
    });

    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.getByText(/E2E Paused Routine/).first()).toBeVisible({ timeout: 10_000 });

    const detailRunRefused = waitForHttp(testPage, "POST", /\/routines\/[^/]+\/run$/, {
      predicate: (r) => r.status() === 409,
    });
    await testPage.getByRole("button", { name: "Run now" }).click();
    await detailRunRefused;
    await expect(testPage.getByText(/Cannot run: routine status is paused/i).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("create dialog persists assignee, policies, task template and arms a cron trigger", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const name = "E2E Wire Contract Create";
    await testPage.goto("/office/routines");
    await testPage.getByRole("button", { name: "New Routine" }).click();

    await testPage.getByLabel("Name").fill(name);
    await testPage
      .getByText("Assignee", { exact: true })
      .locator("..")
      .getByRole("combobox")
      .click();
    await testPage.getByRole("option", { name: "CEO" }).click();
    await testPage.getByRole("button", { name: "Next" }).click();

    await testPage.getByLabel("Task Title Template").fill("{{name}} wire check");
    await testPage.getByLabel("Task Description Template").fill("Verify the wire contract.");
    await testPage.getByRole("button", { name: "Next" }).click();

    await comboboxNear(testPage, "Concurrency").click();
    await testPage.getByRole("option", { name: "Always create" }).click();
    await comboboxNear(testPage, "Catch-up policy").click();
    await testPage.getByRole("option", { name: "Skip missed" }).click();
    await testPage.getByLabel("Cron Expression").fill("*/5 * * * *");
    await testPage.getByLabel("Timezone").fill("America/New_York");

    // AC-OFFICE-ROUTINE-WIRE-002.1/.2: before this capability, the trigger
    // create call this arms was rejected outright (`cronExpression` bound to
    // "" -> `ErrInvalidTrigger` -> 400), so a cron schedule could not be
    // armed from the UI at all.
    const routineCreated = waitForHttp(testPage, "POST", /\/workspaces\/[^/]+\/routines$/);
    const triggerCreated = waitForHttp(testPage, "POST", /\/routines\/[^/]+\/triggers$/);
    await testPage.getByRole("button", { name: "Create" }).click();
    await routineCreated;
    await triggerCreated;
    await expect(testPage.getByText(name)).toBeVisible({ timeout: 10_000 });

    const listed = (await officeApi.listRoutines(officeSeed.workspaceId)) as {
      routines: Record<string, unknown>[];
    };
    const routine = listed.routines.find((r) => r.name === name);
    expect(routine).toBeTruthy();
    // AC-OFFICE-ROUTINE-WIRE-001.1: every multi-word key round-trips under
    // its snake_case wire spelling.
    expect(routine?.assignee_agent_profile_id).toBe(officeSeed.agentId);
    expect(routine?.concurrency_policy).toBe("always_create");
    expect(routine?.catch_up_policy).toBe("skip_missed");
    expect(JSON.parse(routine?.task_template as string)).toEqual({
      title: "{{name}} wire check",
      description: "Verify the wire contract.",
    });

    const triggers = await officeApi.listRoutineTriggers(routine?.id as string);
    const cron = triggers.find((t) => t.kind === "cron");
    expect(cron).toBeTruthy();
    expect(cron?.cron_expression).toBe("*/5 * * * *");
    expect(cron?.timezone).toBe("America/New_York");
    expect(cron?.next_run_at).toBeTruthy();
  });

  test("detail view save persists assignee, policies and arms a cron schedule", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const name = "E2E Wire Contract Detail Save";
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, { name })) as {
      id: string;
    };
    expect(routine.id).toBeTruthy();

    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.getByText(name)).toBeVisible({ timeout: 10_000 });

    await comboboxNear(testPage, "Assignee").click();
    await testPage.getByRole("option", { name: "CEO" }).click();
    await comboboxNear(testPage, "Concurrency policy").click();
    await testPage.getByRole("option", { name: "Always create" }).click();
    await comboboxNear(testPage, "Catch-up policy").click();
    await testPage.getByRole("option", { name: "Skip missed" }).click();
    await textboxNear(testPage, "Cron expression").fill("15 3 * * *");
    await textboxNear(testPage, "Timezone").fill("Europe/London");

    const routineUpdated = waitForHttp(testPage, "PATCH", /\/routines\/[^/]+$/);
    const triggerCreated = waitForHttp(testPage, "POST", /\/routines\/[^/]+\/triggers$/);
    await testPage.getByRole("button", { name: "Save" }).click();
    await routineUpdated;
    await triggerCreated;
    // A successful save with a trigger change calls `router.refresh()`
    // (`window.location.reload()` in this SPA — pre-existing, unchanged by
    // this capability), which races the success toast off the page before
    // Playwright can observe it. Wait for that reload instead of the toast,
    // then read the freshly-seeded form: this is a stronger assertion than
    // the toast anyway, since it also proves the read path round-trips the
    // values this same save just wrote.
    await testPage.waitForLoadState("load");
    await expect(testPage.getByText(name)).toBeVisible({ timeout: 10_000 });
    await expect(comboboxNear(testPage, "Assignee")).toHaveText("CEO");
    await expect(comboboxNear(testPage, "Concurrency policy")).toHaveText("Always create");
    await expect(comboboxNear(testPage, "Catch-up policy")).toHaveText("Skip missed");
    await expect(textboxNear(testPage, "Cron expression")).toHaveValue("15 3 * * *");
    await expect(textboxNear(testPage, "Timezone")).toHaveValue("Europe/London");

    // AC-OFFICE-ROUTINE-WIRE-001.4: an update sends exactly the caller's
    // patch fields under their wire spelling, and the server stores them.
    const stored = await officeApi.getRoutine(routine.id);
    expect(stored.assignee_agent_profile_id).toBe(officeSeed.agentId);
    expect(stored.concurrency_policy).toBe("always_create");
    expect(stored.catch_up_policy).toBe("skip_missed");

    // AC-OFFICE-ROUTINE-WIRE-002.5: the routine had no cron trigger yet, so
    // save creates one and deletes nothing.
    const triggers = await officeApi.listRoutineTriggers(routine.id);
    expect(triggers).toHaveLength(1);
    const cron = triggers.find((t) => t.kind === "cron");
    expect(cron).toBeTruthy();
    expect(cron?.cron_expression).toBe("15 3 * * *");
    expect(cron?.timezone).toBe("Europe/London");
    expect(cron?.next_run_at).toBeTruthy();
  });

  test("list row and detail view render persisted assignee, policy and variables", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const name = "E2E Wire Contract Read";
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name,
      assignee_agent_profile_id: officeSeed.agentId,
      concurrency_policy: "always_create",
      catch_up_policy: "skip_missed",
      // Declared-variable wire shape (backend `parseDeclaredDefaults`,
      // apps/backend/internal/office/routines/service.go:1273): each entry
      // is `{ default: string }`, not a bare scalar.
      variables: JSON.stringify({ region: { default: "us-east" }, tier: { default: "gold" } }),
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    await testPage.goto("/office/routines");
    const row = testPage.getByTestId(`routine-row-${routine.id}`);
    await expect(row).toBeVisible({ timeout: 10_000 });
    // AC-OFFICE-ROUTINE-WIRE-003.1/.6: routine-row.tsx's own snake_case
    // fallbacks are gone; these now render through the normalized model.
    await expect(row.getByText("CEO", { exact: true })).toBeVisible();
    await expect(row.getByText("Always create", { exact: true })).toBeVisible();

    // AC-OFFICE-ROUTINE-WIRE-003.11: one entry per declared variable name,
    // not one entry per character of the encoded JSON string. Also proves
    // Build round 3's fix: a declared variable's `{ default }` object
    // renders its default value, not the literal string "[object Object]".
    await row.click();
    await expect(testPage.getByText("region: us-east")).toBeVisible();
    await expect(testPage.getByText("tier: gold")).toBeVisible();
    await expect(testPage.getByText("[object Object]")).toHaveCount(0);

    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.getByText(name)).toBeVisible({ timeout: 10_000 });
    await expect(comboboxNear(testPage, "Assignee")).toHaveText("CEO");
    await expect(comboboxNear(testPage, "Concurrency policy")).toHaveText("Always create");
    await expect(comboboxNear(testPage, "Catch-up policy")).toHaveText("Skip missed");
  });

  // Review round 1 (Codex-2): handleCreate's post-create refetch used to be
  // unguarded, so a refetch failure became an unhandled rejection that
  // swallowed the create outcome toast instead of surfacing its own. Build
  // round 2 wrapped it in `refreshRoutinesOrReportFailure`; this proves the
  // fix end to end against the real component, not just the unit mock.
  test("a post-create refetch failure reports its own toast and still closes the dialog", async ({
    testPage,
  }) => {
    const name = "E2E Refetch Failure Routine";
    // React StrictMode double-invokes the page-mount effect in dev, so the
    // list endpoint sees two GET calls before any user interaction. Rather
    // than count calls, arm the failure right before the action whose
    // refetch this test targets, so it lands on that call regardless of how
    // many preceded it.
    let failNextList = false;
    await testPage.route("**/api/v1/office/workspaces/*/routines", async (route) => {
      const request = route.request();
      if (request.method() === "GET" && failNextList) {
        failNextList = false;
        await route.fulfill({ status: 500, contentType: "application/json", body: "{}" });
        return;
      }
      await route.continue();
    });

    await testPage.goto("/office/routines");
    await testPage.getByRole("button", { name: "New Routine" }).click();

    await testPage.getByLabel("Name").fill(name);
    await testPage
      .getByText("Assignee", { exact: true })
      .locator("..")
      .getByRole("combobox")
      .click();
    await testPage.getByRole("option", { name: "CEO" }).click();
    await testPage.getByRole("button", { name: "Next" }).click();
    await testPage.getByRole("button", { name: "Next" }).click();

    // Webhook has no required fields at this step, so the create call carries
    // no trigger and this test isolates the routine-list refetch alone.
    await comboboxNear(testPage, "Trigger Type").click();
    await testPage.getByRole("option", { name: "Webhook" }).click();

    const routineCreated = waitForHttp(testPage, "POST", /\/workspaces\/[^/]+\/routines$/);
    failNextList = true;
    const listFailed = waitForHttp(testPage, "GET", /\/workspaces\/[^/]+\/routines$/, {
      predicate: (r) => r.status() === 500,
    });
    await testPage.getByRole("button", { name: "Create" }).click();
    await routineCreated;
    await listFailed;

    await expect(testPage.getByText("Routine created")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Failed to load")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByRole("dialog")).toHaveCount(0);
    expect(failNextList).toBe(false);
  });

  test("manual fire renders the real creation time in the Runs tab", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Wire Contract Run",
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    const fireResponse = await officeApi.runRoutine(routine.id);
    expect(fireResponse.ok).toBe(true);
    const fired = (await fireResponse.json()) as { run: { id: string; created_at: string } };
    expect(fired.run.created_at).toBeTruthy();

    await testPage.goto("/office/routines");
    await testPage.getByRole("tab", { name: "Runs" }).click();
    // officeSeed's workspace is reset per test, so exactly one run exists.
    const runsList = testPage.locator(".rounded-lg.divide-y > div");
    await expect(runsList).toHaveCount(1, { timeout: 10_000 });
    // AC-OFFICE-ROUTINE-WIRE-004.1/.3: `created_at` now reaches the model,
    // so run-row.tsx's `formatTime` renders the real timestamp instead of
    // the "--" placeholder it showed for every run before this capability.
    await expect(runsList).toContainText(new Date(fired.run.created_at).toLocaleString());
  });
});
