import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";

/**
 * Routine catch-up policy rename (gap 24): `enqueue_missed_with_cap` ->
 * `summarize_missed`, with the old value accepted forever as a deprecated
 * alias on write and normalized away on read.
 *
 * The catch-up-policy controls below are still proven through pure
 * client-side interaction (matching AC-003.8's own "component test, not a
 * flow" framing) — that needs no server round trip and stays as-is.
 *
 * HISTORICAL NOTE, now closed: `apps/web/lib/state/slices/office/types.ts`'s
 * `Routine` type declared every multi-word field in camelCase while the
 * backend bound and serialized all of them snake_case, so neither the
 * Create Routine dialog's submit nor the detail view's Save round-tripped
 * any of those fields, and a real "seed via API, load the page, expect the
 * persisted policy to render" test was not possible for either surface.
 * `docs/specs/office/requirements/routine-wire-contract.md` fixed the wire
 * boundary; the last test below is that round trip, now that it exists.
 */

const RETIRED_LABEL = /enqueue.?missed.?with.?cap/i;

function catchUpPolicyCombobox(page: Page) {
  return page.getByText("Catch-up policy", { exact: true }).locator("..").getByRole("combobox");
}

// AC-OFFICE-ROUTINE-CATCHUP-003.6: the summarizing policy's own label
// states a single summarized wake, distinct from catch_up_max's label
// (which states the bound is on ticks counted, not runs created).
const SUMMARIZE_MISSED_LABEL = /Summarize missed.*(once|single)/i;

async function assertCatchUpPolicyToggles(page: Page, catchUpMaxInput: () => Promise<void> | void) {
  // Default: summarize_missed, catch-up max visible.
  await expect(catchUpPolicyCombobox(page)).toHaveText(SUMMARIZE_MISSED_LABEL);
  await catchUpMaxInput();

  await catchUpPolicyCombobox(page).click();
  await page.getByRole("option", { name: "Skip missed", exact: true }).click();
  await expect(page.getByText("Catch-up max", { exact: true })).toHaveCount(0);

  await catchUpPolicyCombobox(page).click();
  await page.getByRole("option", { name: SUMMARIZE_MISSED_LABEL, exact: true }).click();
  await catchUpMaxInput();

  await expect(page.getByText(RETIRED_LABEL)).toHaveCount(0);
}

test.describe("Routine catch-up policy UI", () => {
  test.describe.configure({ timeout: 120_000 });

  test("create dialog: catch-up policy control visibility toggles live", async ({
    testPage,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/office/routines");
    await testPage.getByRole("button", { name: "New Routine" }).click();

    await testPage.getByLabel("Name").fill("E2E Catch-up Dialog");
    await testPage
      .getByText("Assignee", { exact: true })
      .locator("..")
      .getByRole("combobox")
      .click();
    await expect(testPage.getByRole("option", { name: "CEO", exact: true })).toBeVisible({
      timeout: 60_000,
    });
    await testPage.getByRole("option", { name: "CEO", exact: true }).click();
    await testPage.getByRole("button", { name: "Next" }).click();
    await testPage.getByRole("button", { name: "Next" }).click();

    await assertCatchUpPolicyToggles(testPage, async () => {
      await expect(testPage.getByLabel("Catch-up max")).toHaveValue("25");
    });
    await prCapture.screenshot("create-dialog-catch-up-policy", {
      caption:
        "Create Routine dialog with the summarize_missed catch-up policy and its catch-up max field",
    });
  });

  test("detail view: catch-up policy control visibility toggles live", async ({
    officeApi,
    officeSeed,
    testPage,
    prCapture,
  }) => {
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Catch-up Detail Toggle",
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.getByText("E2E Catch-up Detail Toggle").last()).toBeVisible({
      timeout: 10_000,
    });

    await assertCatchUpPolicyToggles(testPage, async () => {
      await expect(testPage.getByText("Catch-up max", { exact: true })).toBeVisible();
    });
    await prCapture.screenshot("detail-view-catch-up-policy", {
      caption:
        "Routine detail view with the summarize_missed catch-up policy and its catch-up max field",
    });
  });

  test("API: the deprecated catch-up policy alias is accepted and normalized on read", async ({
    officeApi,
    officeSeed,
    testPage,
  }) => {
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Catch-up Legacy Alias",
      catch_up_policy: "enqueue_missed_with_cap",
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    // Real HTTP round trip against the running backend (not just the Go
    // repository-level unit tests): the deprecated alias was accepted on
    // write and is normalized away on read, per AC-003.1/AC-003.2.
    const stored = await officeApi.getRoutine(routine.id);
    expect(stored["catch_up_policy"]).toBe("summarize_missed");

    // The detail page loads cleanly for a routine created with the alias,
    // and the retired string never leaks into rendered copy.
    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.getByText("E2E Catch-up Legacy Alias")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText(RETIRED_LABEL)).toHaveCount(0);
  });

  test("detail view: seeded non-default catch-up policy renders and Save persists a change", async ({
    officeApi,
    officeSeed,
    testPage,
  }) => {
    // AC-OFFICE-ROUTINE-WIRE-003.1: seeded via the raw snake_case wire key,
    // read back through the fixed adapter rather than the hardcoded
    // `summarize_missed` UI default every routine rendered as before.
    const routine = (await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "E2E Catch-up Wire Round Trip",
      catch_up_policy: "skip_missed",
    })) as { id: string };
    expect(routine.id).toBeTruthy();

    await testPage.goto(`/office/routines/${routine.id}`);
    await expect(testPage.getByText("E2E Catch-up Wire Round Trip")).toBeVisible({
      timeout: 10_000,
    });
    await expect(catchUpPolicyCombobox(testPage)).toHaveText("Skip missed");
    await expect(testPage.getByText("Catch-up max", { exact: true })).toHaveCount(0);

    // AC-OFFICE-ROUTINE-WIRE-001.4: Save round-trips the change back to the
    // server under the wire's snake_case key.
    await catchUpPolicyCombobox(testPage).click();
    await testPage.getByRole("option", { name: SUMMARIZE_MISSED_LABEL }).click();
    await testPage.getByRole("button", { name: "Save" }).click();
    // A successful save calls `router.refresh()` (`window.location.reload()`
    // in this SPA — pre-existing, unchanged by this capability), which races
    // the success toast off the page before it can be observed. Wait for the
    // reload and read the freshly-seeded form instead, which also proves the
    // read path round-trips what this save just wrote.
    await testPage.waitForLoadState("load");
    await expect(testPage.getByText("E2E Catch-up Wire Round Trip")).toBeVisible({
      timeout: 10_000,
    });
    await expect(catchUpPolicyCombobox(testPage)).toHaveText(SUMMARIZE_MISSED_LABEL);

    const stored = await officeApi.getRoutine(routine.id);
    expect(stored["catch_up_policy"]).toBe("summarize_missed");
  });
});
