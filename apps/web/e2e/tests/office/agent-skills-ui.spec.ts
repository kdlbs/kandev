import { test, expect } from "../../fixtures/office-fixture";

/**
 * Agent skills tab — UI hydration on direct navigation.
 *
 * Regression context: the tab reads `state.office.skills` from the
 * Zustand store. The workspace Skills page populates that slice as
 * a side effect of viewing, so a user landing directly on
 * `/office/agents/<id>/skills` (e.g. from a bookmark or the run-detail
 * deep link) used to see "No skills registered yet" even when the
 * workspace had 13 system skills. The tab now hydrates itself via
 * `listSkills` on mount; these specs pin that behaviour so a future
 * refactor doesn't regress it again.
 */
test.describe("Agent skills tab UI", () => {
  test("direct navigation hydrates the skill list (no prior visit to Skills page)", async ({
    testPage,
    officeSeed,
  }) => {
    // Hit the Skills sub-route straight away — no detour through
    // /office/workspace/skills first. The agent slug-cum-page also
    // forces a fresh store on every test via the fixture's e2eReset.
    await testPage.goto(`/office/agents/${officeSeed.agentId}/skills`);

    // The Skills tab is selected.
    await expect(testPage.getByTestId("agent-tab-skills")).toBeVisible({ timeout: 10_000 });

    // The skill list renders with at least one checkbox (rather than
    // the "No skills registered" CTA that surfaces when the store is
    // empty). Use the first system-skill name as a sentinel; if the
    // bundled set ever changes, prefer the count assertion below.
    await expect(testPage.getByText("kandev-protocol").first()).toBeVisible({ timeout: 10_000 });

    // The CEO is the seeded agent and inherits ceo-default skills, so
    // multiple rows should be checked already. We assert at least one
    // checked checkbox to confirm the desired_skills round-trip.
    const checkedRows = testPage.locator('[data-state="checked"]');
    await expect(checkedRows.first()).toBeVisible({ timeout: 5_000 });
  });

  test("system skills carry the System badge alongside the slug", async ({
    testPage,
    officeSeed,
  }) => {
    await testPage.goto(`/office/agents/${officeSeed.agentId}/skills`);
    await expect(testPage.getByText("kandev-protocol").first()).toBeVisible({ timeout: 10_000 });
    // The badge is rendered with the literal text "System" inline with
    // each system row. We only assert at least one badge — the count
    // matches the bundled-set size, which is volatile.
    await expect(testPage.getByText(/^System$/).first()).toBeVisible({ timeout: 5_000 });
  });

  test("toggling a system skill persists across reload", async ({ testPage, officeSeed }) => {
    // Regression: confirms the full click → PATCH → store → reload
    // round-trip for the per-agent skill toggle. The hydration spec
    // above only proves the list renders; this one proves the write
    // path is wired up to the AgentProfile PATCH endpoint and that
    // the toggle state rehydrates from the server after a reload.
    await testPage.goto(`/office/agents/${officeSeed.agentId}/skills`);
    await expect(testPage.getByTestId("agent-tab-skills")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("kandev-protocol").first()).toBeVisible({ timeout: 10_000 });

    // The CEO has kandev-protocol auto-attached as a default system
    // skill, so the toggle should start checked.
    const row = testPage.getByTestId("skill-toggle-kandev-protocol");
    await expect(row).toBeVisible();
    const checkbox = row.locator('button[role="checkbox"]');
    await expect(checkbox).toHaveAttribute("data-state", "checked", { timeout: 5_000 });

    // Untick, hit Save, wait for the PATCH /api/v1/office/agents/<id>
    // to complete with 200. This is the same convention the
    // onboarding spec uses for write-path settling.
    await checkbox.click();
    await expect(checkbox).toHaveAttribute("data-state", "unchecked");
    const saveResponse = testPage.waitForResponse(
      (resp) =>
        resp.url().includes(`/api/v1/office/agents/${officeSeed.agentId}`) &&
        resp.request().method() === "PATCH" &&
        resp.status() === 200,
      { timeout: 10_000 },
    );
    await testPage.getByRole("button", { name: /save skills/i }).click();
    await saveResponse;

    // Reload and re-assert from a cold store. The toggle should still
    // be unchecked, proving the PATCH persisted and the hydration
    // round-trips desiredSkillIds.
    await testPage.reload();
    await expect(testPage.getByText("kandev-protocol").first()).toBeVisible({ timeout: 10_000 });
    const rowAfterReload = testPage.getByTestId("skill-toggle-kandev-protocol");
    const checkboxAfterReload = rowAfterReload.locator('button[role="checkbox"]');
    await expect(checkboxAfterReload).toHaveAttribute("data-state", "unchecked", {
      timeout: 5_000,
    });

    // Tidy up worker-scoped state so retries / subsequent tests in
    // this worker start from the same "kandev-protocol attached"
    // baseline the onboarding fixture established.
    await checkboxAfterReload.click();
    await expect(checkboxAfterReload).toHaveAttribute("data-state", "checked");
    const restoreResponse = testPage.waitForResponse(
      (resp) =>
        resp.url().includes(`/api/v1/office/agents/${officeSeed.agentId}`) &&
        resp.request().method() === "PATCH" &&
        resp.status() === 200,
      { timeout: 10_000 },
    );
    await testPage.getByRole("button", { name: /save skills/i }).click();
    await restoreResponse;
  });
});
