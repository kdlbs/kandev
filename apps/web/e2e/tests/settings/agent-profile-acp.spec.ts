import { test, expect } from "../../fixtures/test-base";
import { injectLatency } from "../../helpers/causal-waits";
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
    // Provider registration order is not a stable contract. The mock agent is
    // the fixture that advertises deterministic ACP modes, so select it by id
    // instead of relying on the first row returned by the API.
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
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

  test("profile model picker opens from the field start edge", async ({ testPage, apiClient }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
    const profile = agent.profiles[0];

    await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

    const profileSettingsPanel = testPage.locator(
      '[data-settings-target^="setting-agent-profile-"][data-settings-target$="-profile-settings"]',
    );
    const selector = profileSettingsPanel.getByRole("button", {
      name: "Profile start model settings",
    });
    await expect(selector).toBeVisible({ timeout: 15_000 });
    await selector.click();

    const popoverContent = testPage.locator('[data-slot="popover-content"]:visible');
    await expect(popoverContent).toBeVisible();
    await popoverContent.evaluate(async (element) => {
      await Promise.all(
        element.getAnimations().map((animation) => animation.finished.catch(() => undefined)),
      );
    });

    const [selectorBox, popoverBox] = await Promise.all([
      selector.boundingBox(),
      popoverContent.boundingBox(),
    ]);

    expect(selectorBox).not.toBeNull();
    expect(popoverBox).not.toBeNull();
    // Radix may nudge the content by a few pixels to satisfy collision padding;
    // the regression is the old end alignment, which placed it ~50px inward.
    expect(Math.abs(popoverBox!.x - selectorBox!.x)).toBeLessThanOrEqual(12);
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
      await testPage.route("**/api/v1/agent-models/mock-agent/resolve", async (route) => {
        const request = route.request().postDataJSON() as { model?: string };
        if (request.model === "mock-smart") {
          await injectLatency(
            750,
            "keeps the profile model-option loading state visible during resolution",
          );
        }
        await route.fallback();
      });
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeVisible({ timeout: 15_000 });
      // Shared profile selectors retain the all-values summary; baseline
      // compaction applies only to task chat.
      await expect(selector).toHaveText("Mock Fast / High", { timeout: 10_000 });
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden({
        timeout: 15_000,
      });
      await expect(testPage.getByTestId("profile-refresh-capabilities")).toBeEnabled({
        timeout: 15_000,
      });

      await selector.click();
      await expect(
        testPage.getByText("Fast mock model for testing", { exact: true }),
      ).toBeVisible();
      const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
      await expect(effortTrigger).toBeVisible();
      await effortTrigger.click();
      await testPage.getByRole("button", { name: "Low", exact: true }).click();
      await expect(selector).toHaveText("Mock Fast / Low");

      // Changing the model must replace the option snapshot. mock-smart
      // exposes Max while mock-fast exposes Medium, so this also proves the
      // profile selector does not keep the previous model's option list.
      await testPage.getByRole("option", { name: /Mock Smart/ }).click();
      await expect(testPage.getByTestId("model-config-options-loading")).toBeVisible();
      const selectedModelRow = testPage.getByTestId("model-config-selected-row");
      await expect(selectedModelRow.locator("svg.tabler-icon-loader")).toBeVisible();
      await expect(selectedModelRow.locator("svg.tabler-icon-check.absolute")).toHaveCount(0);
      await expect(testPage.getByTestId("config-option-trigger-effort")).toHaveCount(0);
      await expect(selector).toHaveAttribute("aria-expanded", "true");
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden({
        timeout: 15_000,
      });
      await expect(testPage.getByTestId("model-config-options-loading")).toHaveCount(0);
      await expect(selectedModelRow.locator("svg.tabler-icon-loader")).toHaveCount(0);
      await expect(selectedModelRow.locator("svg.tabler-icon-check.absolute")).toBeVisible();
      await expect(selector).toContainText("Mock Smart", { timeout: 10_000 });
      await expect(testPage.getByTestId("config-option-trigger-effort")).toBeVisible();
      await testPage.getByTestId("config-option-trigger-effort").click();
      await expect(testPage.getByRole("button", { name: "Max", exact: true })).toBeVisible();
      await testPage.getByRole("button", { name: "Max", exact: true }).click();
      await expect(selector).toHaveText("Mock Smart / Max");

      // Restore the original model before exercising persistence below.
      await testPage.getByRole("option", { name: /Mock Fast/ }).click();
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden({
        timeout: 15_000,
      });
      await testPage.getByTestId("config-option-trigger-effort").click();
      await testPage.getByRole("button", { name: "Low", exact: true }).click();
      await expect(selector).toHaveText("Mock Fast / Low");
      await selector.click();

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
      await expect(selector).toHaveText("Mock Fast / Low", { timeout: 15_000 });
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
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
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
