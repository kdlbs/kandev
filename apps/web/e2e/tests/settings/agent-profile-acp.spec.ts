import { test, expect } from "../../fixtures/test-base";

/**
 * Verifies the ACP-first profile editor:
 *
 * - Legacy permission toggles (`auto_approve`, `dangerously_skip_permissions`)
 *   are no longer rendered. They were removed when profile permission stance
 *   moved to ACP session modes + per-tool-call permission_request prompts.
 * - Profile name edits persist across reload (exercises the new AgentProfile
 *   DTO shape with `mode` / `migrated_from` columns).
 * - Mode picker is hidden when the agent's capability cache has no modes —
 *   the mock agent in E2E isn't probed so no mode data flows through.
 */
test.describe("Agent profile — ACP-first", () => {
  test("profile editor loads with model picker and without legacy permission toggles", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profileId = agent.profiles[0].id;

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profileId}`);

    // Profile name input is present (from the shared ProfileFormFields component).
    await expect(testPage.getByTestId("profile-name-input")).toBeVisible({ timeout: 15_000 });

    // The old permission toggle labels must NOT render. These were backed by
    // the removed AgentProfile.auto_approve and .dangerously_skip_permissions
    // fields; the corresponding PermissionSetting entries are gone from the
    // shared agents so the PermissionToggles iterator emits nothing for them.
    await expect(testPage.getByText(/Auto-approve/i)).toHaveCount(0);
    await expect(testPage.getByText(/YOLO/i)).toHaveCount(0);
    await expect(testPage.getByText(/Skip Permissions/i)).toHaveCount(0);
    await expect(testPage.getByText(/dangerously skip/i)).toHaveCount(0);

    // The mock agent isn't an InferenceAgent, so the host utility cache has no
    // modes for it and the mode picker is not rendered.
    await expect(testPage.getByTestId("profile-mode-field")).toHaveCount(0);
  });

  test("profile name edits persist across reload", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    // Use the seeded default profile rather than creating a new one — the
    // profile editor page reads from the agents list that's hydrated on the
    // server, and a freshly POSTed profile race-conditions with SSR.
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = agent.profiles[0];
    const originalName = profile.name;

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

    const nameInput = testPage.getByTestId("profile-name-input");
    await expect(nameInput).toBeVisible({ timeout: 15_000 });
    await expect(nameInput).toHaveValue(originalName, { timeout: 10_000 });

    // Edit name.
    const newName = `${originalName} Renamed`;
    await nameInput.fill(newName);

    // Save via the dirty-state save button (card header). The save dispatches
    // a Next.js server action, so we wait for the dirty badge to disappear
    // as the signal that the round-trip completed.
    const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
    await expect(saveButton).toBeEnabled({ timeout: 10_000 });
    await saveButton.click();
    await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

    // Reload and assert the new name persisted — this exercises the round-trip
    // through the new profile DTO shape (model + mode + allow_indexing +
    // cli_passthrough) without the legacy permission columns.
    await testPage.reload();
    await expect(testPage.getByTestId("profile-name-input")).toHaveValue(newName, {
      timeout: 15_000,
    });

    // Restore the original name via the API so the worker-scoped seedData
    // fixture stays valid for subsequent tests.
    await apiClient.updateAgentProfile(profile.id, { name: originalName });
  });
});
