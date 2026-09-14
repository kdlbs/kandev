import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/office-fixture";

/**
 * Routine catch-up policy rename (gap 24): `enqueue_missed_with_cap` ->
 * `summarize_missed`, with the old value accepted forever as a deprecated
 * alias on write and normalized away on read.
 *
 * NOTE ON SCOPE: `apps/web/lib/state/slices/office/types.ts`'s `Routine`
 * type (pre-existing, unchanged by this diff) declares every multi-word
 * field in camelCase (`catchUpPolicy`, `catchUpMax`, `concurrencyPolicy`,
 * `assigneeAgentProfileId`, `taskTemplate`, `workspaceId`), but the backend
 * (`apps/backend/internal/office/models/models.go`) serializes and binds
 * all of them in snake_case. Neither the Create Routine dialog's submit
 * nor the detail view's Save round-trips any of those fields — they're
 * silently dropped on write and read back as `undefined` (falling through
 * to each field's hardcoded UI default) regardless of what's actually
 * persisted. This is pre-existing (present unchanged at the merge base)
 * and spans the whole Routines feature, not just catch-up policy — filed
 * as a follow-up, not fixed here. Consequence for this spec: the create
 * dialog's and detail view's catch-up-policy controls can only be proven
 * correct through pure client-side interaction (matching AC-003.8's own
 * "component test, not a flow" framing) — a real "seed via API, load the
 * page, expect the persisted policy to render" round trip is not currently
 * possible for either surface. The alias-normalization contract itself
 * (AC-003.1) is still verified for real over HTTP, via the API layer.
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
  await page.getByRole("option", { name: "Skip missed" }).click();
  await expect(page.getByText("Catch-up max", { exact: true })).toHaveCount(0);

  await catchUpPolicyCombobox(page).click();
  await page.getByRole("option", { name: SUMMARIZE_MISSED_LABEL }).click();
  await catchUpMaxInput();

  await expect(page.getByText(RETIRED_LABEL)).toHaveCount(0);
}

test.describe("Routine catch-up policy UI", () => {
  test("create dialog: catch-up policy control visibility toggles live", async ({
    testPage,
    prCapture,
  }) => {
    await testPage.goto("/office/routines");
    await testPage.getByRole("button", { name: "New Routine" }).click();

    await testPage.getByLabel("Name").fill("E2E Catch-up Dialog");
    await testPage
      .getByText("Assignee", { exact: true })
      .locator("..")
      .getByRole("combobox")
      .click();
    await testPage.getByRole("option", { name: "CEO" }).click();
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
    await expect(testPage.getByText("E2E Catch-up Detail Toggle")).toBeVisible({
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
});
