import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Verifies the ACP-first profile editor:
 *
 * - Universal agentctl auto-approve toggle renders with danger styling.
 * - Codex no longer renders stale `-c` config toggles unsupported by the ACP bridge.
 * - Profile name edits persist across reload (exercises the new AgentProfile
 *   DTO shape with `mode` / `migrated_from` columns).
 * - Mode picker renders when the agent's capability cache advertises modes.
 *   The E2E mock agent advertises modes in NewSession so the picker appears.
 * - Profile mode propagates to the session UI: mode selector visible with the
 *   correct active mode after launching a task with a non-default profile mode.
 */
test.describe("Agent profile — ACP-first", () => {
  test("profile editor loads with model picker and permission toggles", async ({
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

    await expect(testPage.getByTestId("permission-auto-approve-danger")).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByText(/Skip Permissions/i)).toHaveCount(0);
    await expect(testPage.getByText(/dangerously skip/i)).toHaveCount(0);

    if (agent.name === "codex-acp") {
      await expect(
        testPage.getByTestId("cli-flag-curated-config_approval_policy_never"),
      ).toHaveCount(0);
      await expect(
        testPage.getByTestId("cli-flag-curated-config_sandbox_disk_full_read"),
      ).toHaveCount(0);
    }

    // The mock agent advertises modes, so the mode picker is rendered.
    await expect(testPage.getByTestId("profile-mode-field")).toBeVisible({ timeout: 10_000 });
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

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const nameInput = testPage.getByTestId("profile-name-input");
      await expect(nameInput).toBeVisible({ timeout: 15_000 });
      await expect(nameInput).toHaveValue(originalName, { timeout: 10_000 });

      // Edit name.
      const newName = `${originalName} Renamed`;
      await nameInput.fill(newName);

      // Save via the dirty-state save button (card header). The save dispatches
      // an action wrapper, so we wait for the dirty badge to disappear as the
      // signal that the round-trip completed.
      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      // Reload and assert the new name persisted — this exercises the
      // round-trip through the new profile DTO shape (model + mode +
      // allow_indexing + cli_passthrough) without the legacy permission columns.
      await testPage.reload();
      await expect(testPage.getByTestId("profile-name-input")).toHaveValue(newName, {
        timeout: 15_000,
      });
    } finally {
      // Always restore the original name so the worker-scoped seedData
      // fixture stays valid for subsequent tests — even if an assertion
      // above failed.
      await apiClient.updateAgentProfile(profile.id, { name: originalName });
    }
  });

  test("profile model selector shows and saves dynamic config options", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(60_000);

    await expect
      .poll(
        async () => {
          const resp = await testPage.request.get(`${backend.baseUrl}/api/v1/agents/available`);
          if (!resp.ok()) return false;
          const data = (await resp.json()) as {
            agents?: {
              name: string;
              model_config?: { config_options?: { id: string }[] };
            }[];
          };
          const mock = data.agents?.find((a) => a.name === "mock-agent");
          return Boolean(
            mock?.model_config?.config_options?.some((option) => option.id === "effort"),
          );
        },
        { timeout: 20_000, intervals: [250, 500, 1000] },
      )
      .toBe(true);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Config Option Test Profile", {
      model: "mock-fast",
      config_options: { effort: "high" },
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeVisible({ timeout: 15_000 });
      await expect(selector).toContainText("High", { timeout: 10_000 });

      await selector.click();
      const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
      await expect(effortTrigger).toBeVisible();
      await effortTrigger.click();
      await testPage.getByRole("button", { name: "Low", exact: true }).click();
      await expect(selector).toContainText("Low");

      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      await expect
        .poll(
          async () => {
            const saved = (await apiClient.getAgentProfile(profile.id)) as unknown as {
              configOptions?: Record<string, string>;
              config_options?: Record<string, string>;
            };
            return saved.configOptions?.effort ?? saved.config_options?.effort ?? "";
          },
          { timeout: 10_000, intervals: [250, 500, 1000] },
        )
        .toBe("low");

      await testPage.reload();
      await expect(selector).toContainText("Low", { timeout: 15_000 });
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("profile mode propagates to session mode selector", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT", "REVIEW"];

    // 1. Get mock agent and create a profile with mode "plan-mock"
    const { agents } = await apiClient.listAgents();
    const agent = agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Mode Test Profile", {
      model: "mock-fast",
      mode: "plan-mock",
    });

    try {
      // 2. Create task using the profile with mode set
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Mode Selector Test",
        profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );

      // 3. Wait for session to finish its first turn
      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(task.id);
            return DONE_STATES.includes(sessions[0]?.state ?? "");
          },
          { timeout: 30_000, message: "Waiting for session to finish" },
        )
        .toBe(true);

      // 4. Navigate to the task session
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 45_000 });

      // 5. Assert the mode selector is visible and shows the profile mode.
      const overflowToggle = testPage.getByTestId("toolbar-overflow-menu");
      if (await overflowToggle.isVisible({ timeout: 1_000 }).catch(() => false)) {
        await overflowToggle.click();
      }
      const modeSelector = testPage.getByRole("button", { name: "Plan Mock" });
      await expect(modeSelector).toBeVisible({ timeout: 15_000 });
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
