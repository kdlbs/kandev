import { test, expect } from "../../fixtures/test-base";

/**
 * Covers the Utility Agents settings page.
 *
 * The first test is a regression guard for a bug where the backend emitted
 * `models: null` on /api/v1/utility/inference-agents. The frontend's flatMap
 * over `ia.models` blew up and crashed the whole settings page during render.
 * The other tests smoke-check the page loads and walk through the main
 * interactions (open the page, inspect sections, open the create dialog).
 */
test.describe("Utility Agents settings page", () => {
  test("repairs an unavailable action with the default", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const agentId = "builtin-commit-message";
    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((agent) => agent.profiles ?? [])
      .find((candidate) => candidate.id === seedData.agentProfileId);
    if (!profile) throw new Error(`seed profile ${seedData.agentProfileId} was not found`);
    const profileLabel = `${profile.agentDisplayName} • ${profile.name}`;
    const baselineResponse = await apiClient.rawRequest("GET", `/api/v1/utility/agents/${agentId}`);
    expect(baselineResponse.ok).toBe(true);
    const baseline = await baselineResponse.json();

    await testPage.route("**/api/v1/user/settings", (route) => {
      if (route.request().method() !== "GET") return route.continue();
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: { default_utility_agent_profile_id: seedData.agentProfileId },
        }),
      });
    });
    await testPage.route("**/api/v1/utility/agents", (route) => {
      if (route.request().method() !== "GET") return route.continue();
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              ...baseline,
              agent_profile_id: "",
              profile_binding_state: "unconfigured",
              enabled: false,
            },
          ],
        }),
      });
    });

    try {
      await testPage.goto("/settings/utility-agents");
      await expect(
        testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
      ).toBeVisible({ timeout: 15_000 });

      const row = testPage.getByTestId(`utility-action-row-${agentId}`);
      const picker = row.getByTestId(`utility-profile-picker-action-${agentId}`);
      await expect(
        row.getByText("This profile is unavailable. Select an enabled inference profile.", {
          exact: true,
        }),
      ).toBeVisible();

      await picker.click();
      const listbox = testPage.getByRole("listbox");
      await listbox.locator('[data-value="__USE_DEFAULT__"]').click();

      const floatingSave = testPage.getByTestId("settings-floating-save");
      await expect(floatingSave).toBeVisible();
      await floatingSave.getByRole("button", { name: "Save changes" }).click();
      await expect(floatingSave).not.toBeVisible({ timeout: 10_000 });

      await testPage.unroute("**/api/v1/utility/agents");
      await testPage.reload();
      await expect(picker).toContainText(profileLabel, { timeout: 15_000 });
      await expect(
        row.getByText("This profile is unavailable. Select an enabled inference profile.", {
          exact: true,
        }),
      ).toHaveCount(0);
      await prCapture.screenshot("utility-action-inherits-default-desktop", {
        caption: "The repaired utility action inherits the selected default profile.",
      });

      const persistedResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/utility/agents/${agentId}`,
      );
      expect(persistedResponse.ok).toBe(true);
      const persisted = await persistedResponse.json();
      expect(persisted).toMatchObject({
        agent_profile_id: "",
        profile_binding_state: "inherit",
        enabled: true,
      });
    } finally {
      const restoreResponse = await apiClient.rawRequest(
        "PATCH",
        `/api/v1/utility/agents/${agentId}`,
        {
          agent_profile_id: baseline.agent_profile_id,
          profile_binding_state: baseline.profile_binding_state,
          enabled: baseline.enabled,
        },
      );
      expect(restoreResponse.ok).toBe(true);
    }
  });

  test("renders the default for an inherited built-in action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((agent) => agent.profiles ?? [])
      .find((candidate) => candidate.id === seedData.agentProfileId);
    if (!profile) throw new Error(`seed profile ${seedData.agentProfileId} was not found`);
    const profileLabel = `${profile.agentDisplayName} • ${profile.name}`;

    await testPage.route("**/api/v1/user/settings", (route) => {
      if (route.request().method() !== "GET") return route.continue();
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          settings: { default_utility_agent_profile_id: seedData.agentProfileId },
        }),
      });
    });
    await testPage.route("**/api/v1/utility/agents", (route) => {
      if (route.request().method() !== "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ success: true }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "builtin-commit-message",
              name: "commit-message",
              description: "Generate a commit message.",
              builtin: true,
              enabled: true,
              agent_id: "",
              model: "",
              agent_profile_id: "",
              profile_binding_state: "inherit",
            },
          ],
        }),
      });
    });

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    const row = testPage.getByTestId("utility-action-row-builtin-commit-message");
    await expect(
      row.getByTestId("utility-profile-picker-action-builtin-commit-message"),
    ).toContainText(profileLabel);
    await expect(
      row.getByText("This profile is unavailable. Select an enabled inference profile.", {
        exact: true,
      }),
    ).toHaveCount(0);
  });

  test("profile picker keeps wheel scrolling inside the menu", async ({ testPage }) => {
    const models = Array.from({ length: 36 }, (_, index) => ({
      id: `claude-model-${String(index + 1).padStart(2, "0")}`,
      name: `Claude Model ${String(index + 1).padStart(2, "0")}`,
      description: "",
      is_default: index === 0,
    }));

    await testPage.route("**/api/v1/utility/inference-agents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "claude",
              name: "claude",
              display_name: "Claude",
              status: "ok",
              models,
            },
          ],
        }),
      }),
    );
    await testPage.route("**/api/v1/utility/agents", (route) => {
      if (route.request().method() !== "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ success: true }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "builtin-commit-message",
              name: "commit-message",
              description: "Generate a commit message.",
              builtin: true,
              enabled: true,
              agent_id: "",
              model: "",
            },
          ],
        }),
      });
    });

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    await testPage.evaluate(() => {
      const testWindow = window as Window & { __utilitySelectWheelEvents?: number };
      testWindow.__utilitySelectWheelEvents = 0;
      document.addEventListener("wheel", (event) => {
        const target = event.target;
        if (target instanceof Element && target.closest('[data-slot="popover-content"]')) {
          testWindow.__utilitySelectWheelEvents = (testWindow.__utilitySelectWheelEvents ?? 0) + 1;
        }
      });
    });

    const actionRow = testPage.getByTestId("utility-action-row-builtin-commit-message");
    const actionSelect = actionRow.getByRole("combobox");
    await actionSelect.click();
    const popoverContent = testPage.locator('[data-slot="popover-content"]').first();
    await expect(popoverContent).toBeVisible();
    await popoverContent.evaluate((element) => {
      const testWindow = window as Window & { __utilitySelectRawWheelEvents?: number };
      testWindow.__utilitySelectRawWheelEvents = 0;
      element.addEventListener("wheel", () => {
        testWindow.__utilitySelectRawWheelEvents =
          (testWindow.__utilitySelectRawWheelEvents ?? 0) + 1;
      });
    });

    await popoverContent.dispatchEvent("wheel", {
      bubbles: true,
      cancelable: true,
      deltaY: 900,
    });

    await expect
      .poll(() =>
        testPage.evaluate(
          () =>
            (window as Window & { __utilitySelectWheelEvents?: number })
              .__utilitySelectWheelEvents ?? 0,
        ),
      )
      .toBe(0);
    await expect
      .poll(() =>
        testPage.evaluate(
          () =>
            (window as Window & { __utilitySelectRawWheelEvents?: number })
              .__utilitySelectRawWheelEvents ?? 0,
        ),
      )
      .toBe(1);
  });

  test("does not crash when backend returns models: null", async ({ testPage }) => {
    // Simulate the exact shape the backend used to emit. Guards against a
    // regression where frontend null-deref would take the whole page down.
    await testPage.route("**/api/v1/utility/inference-agents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "broken-agent",
              name: "broken-agent",
              display_name: "Broken Agent",
              models: null,
            },
          ],
        }),
      }),
    );

    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    await testPage.goto("/settings/utility-agents");

    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });

  test("renders all sections with seeded built-in utility agents", async ({ testPage }) => {
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    await testPage.goto("/settings/utility-agents");

    // Top-level heading + subtitle.
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      testPage.getByText(
        "One-shot helpers run Kandev UI actions such as commit and PR text generation or prompt enhancement. They are separate from agents that work inside task sessions.",
      ),
    ).toBeVisible();

    // Default-model section.
    await expect(testPage.getByText("Default utility agent model", { exact: true })).toBeVisible();

    // Built-in actions (seeded on first boot — see builtins.go).
    // Assert a representative subset; the full list lives server-side.
    await expect(testPage.getByText("commit-message", { exact: true })).toBeVisible();
    await expect(testPage.getByText("pr-title", { exact: true })).toBeVisible();
    await expect(testPage.getByText("enhance-prompt", { exact: true })).toBeVisible();

    // Custom agents section + empty state.
    await expect(testPage.getByText("Custom utility agents", { exact: true })).toBeVisible();
    await expect(testPage.getByText("No custom utility agents.")).toBeVisible();

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });

  test("explains utility actions and filters profile pickers with agent icons", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((agent) => agent.profiles ?? [])
      .find((candidate) => candidate.id === seedData.agentProfileId);
    if (!profile) throw new Error(`seed profile ${seedData.agentProfileId} was not found`);
    const profileLabel = `${profile.agentDisplayName} • ${profile.name}`;

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    await expect(
      testPage.getByText(
        "One-shot helpers run Kandev UI actions such as commit and PR text generation or prompt enhancement. They are separate from agents that work inside task sessions.",
      ),
    ).toBeVisible();
    await expect(
      testPage.getByText(
        "Override the profile for a specific Kandev UI action. These actions do not configure agents that run inside task sessions.",
      ),
    ).toBeVisible();

    const cardOrder = await testPage.evaluate(() => {
      const ids = [
        "utility-default-model-card",
        "config-chat-agent-card",
        "utility-actions-card",
        "utility-custom-agents-card",
      ];
      const cards = ids.map((id) => document.querySelector(`[data-testid="${id}"]`));
      return cards.every(
        (card, index) =>
          card !== null &&
          cards
            .slice(index + 1)
            .every(
              (next) =>
                next !== null &&
                Boolean(card.compareDocumentPosition(next) & Node.DOCUMENT_POSITION_FOLLOWING),
            ),
      );
    });
    expect(cardOrder).toBe(true);

    const picker = testPage.getByTestId("utility-profile-picker-default");
    const configPicker = testPage.getByTestId("utility-profile-picker-config-chat");
    await expect(picker).toHaveClass(/border-border/);
    await expect(configPicker).toHaveClass(/border-border/);
    await expect(
      testPage.getByTestId("config-chat-agent-card").getByText("Agent profile", { exact: true }),
    ).toBeVisible();
    const [defaultPickerBox, configPickerBox] = await Promise.all([
      picker.boundingBox(),
      configPicker.boundingBox(),
    ]);
    expect(defaultPickerBox).not.toBeNull();
    expect(configPickerBox).not.toBeNull();
    expect(defaultPickerBox!.width).toBeCloseTo(configPickerBox!.width, 0);

    const cardTypography = await testPage
      .locator('[data-slot="card-header"] h3')
      .evaluateAll((elements) =>
        elements.map((element) => {
          const style = getComputedStyle(element);
          return {
            fontFamily: style.fontFamily,
            fontSize: style.fontSize,
            lineHeight: style.lineHeight,
          };
        }),
      );
    expect(new Set(cardTypography.map((style) => JSON.stringify(style))).size).toBe(1);

    await picker.click();
    const listbox = testPage.getByRole("listbox");
    await expect(listbox).toBeVisible();
    await testPage.getByPlaceholder("Search agent profiles...").fill(profile.agentDisplayName);
    const option = listbox.getByRole("option", { name: profileLabel, exact: true });
    await expect(option).toBeVisible();
    await expect(option.getByTestId("utility-profile-agent-icon")).toBeVisible();
    await option.click();
    await expect(picker).toContainText(profileLabel);
    await testPage.keyboard.press("Escape");

    await configPicker.click();
    const configDropdown = testPage.getByTestId("utility-profile-picker-config-chat-dropdown");
    const configListbox = configDropdown.getByRole("listbox");
    await testPage
      .getByTestId("utility-profile-picker-config-chat-dropdown")
      .getByPlaceholder("Search agent profiles...")
      .fill(profile.name);
    await expect(
      configListbox.getByRole("option", { name: profileLabel, exact: true }),
    ).toBeVisible();
    await expect(
      configListbox
        .getByRole("option", { name: profileLabel, exact: true })
        .getByTestId("utility-profile-agent-icon"),
    ).toBeVisible();
  });

  test("wraps each settings sub-section in a bounded card", async ({ testPage }) => {
    // Regression guard: these sub-sections used to be bare divs with no
    // visible border, unlike every other settings page (whose
    // Card-wrapped Enable/Engine/Behavior sections).
    //
    // Mock the inference-agent + builtin-agent APIs so the Actions card
    // (which renders null when there are no builtins — see
    // PerActionOverridesSection) is deterministically present, instead of
    // depending on the backend's boot-time seed state.
    await testPage.route("**/api/v1/utility/inference-agents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "claude",
              name: "claude",
              display_name: "Claude",
              status: "ok",
              models: [
                { id: "claude-fast", name: "Claude Fast", description: "", is_default: true },
              ],
            },
          ],
        }),
      }),
    );
    await testPage.route("**/api/v1/utility/agents", (route) => {
      if (route.request().method() !== "GET") {
        return route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ success: true }),
        });
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "builtin-commit-message",
              name: "commit-message",
              description: "Generate a commit message.",
              builtin: true,
              enabled: true,
              agent_id: "",
              model: "",
            },
          ],
        }),
      });
    });

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    for (const { testId, heading } of [
      { testId: "utility-default-model-card", heading: "Default utility agent model" },
      { testId: "utility-actions-card", heading: "Actions" },
      { testId: "utility-custom-agents-card", heading: "Custom utility agents" },
      { testId: "config-chat-agent-card", heading: "Configuration Chat Agent" },
    ]) {
      const card = testPage.getByTestId(testId);
      await expect(card).toBeVisible();
      await expect(card).toHaveAttribute("data-slot", "card");
      // Card titles must stay real headings (level 3), not just styled div
      // text, so screen-reader heading navigation keeps working.
      await expect(
        card.getByRole("heading", { name: heading, level: 3, exact: true }),
      ).toBeVisible();
    }
  });

  test("opens the create-agent dialog from the Add button", async ({ testPage }) => {
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    await testPage.goto("/settings/utility-agents");

    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    await testPage.getByRole("button", { name: "Add", exact: true }).click();

    // The dialog is rendered by UtilityAgentDialog; title differs between
    // create and edit mode. We're in create mode here.
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByText("Create Utility Agent")).toBeVisible();

    // Close the dialog — nothing should explode.
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).not.toBeVisible();

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });

  test("selecting a profile configures the default utility agent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Utility agents now use the same persisted agent profiles as task
    // sessions. Keep this regression guard on the profile selector so a
    // future change cannot reintroduce the legacy agent/model pair.
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    const { agents } = await apiClient.listAgents();
    const profile = agents
      .flatMap((agent) => agent.profiles ?? [])
      .find((candidate) => candidate.id === seedData.agentProfileId);
    if (!profile) throw new Error(`seed profile ${seedData.agentProfileId} was not found`);
    const profileLabel = `${profile.agentDisplayName} • ${profile.name}`;

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    const profileSelect = testPage.getByTestId("utility-default-model-card").getByRole("combobox");
    await profileSelect.click();
    const agentListbox = testPage.getByRole("listbox");
    await expect(agentListbox).toBeVisible();
    await expect(
      agentListbox.getByRole("option", { name: profileLabel, exact: true }),
    ).toBeVisible();
    await agentListbox.getByRole("option", { name: profileLabel, exact: true }).click();
    await expect(agentListbox).not.toBeVisible();
    await expect(profileSelect).toContainText(profileLabel);

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });

  test("Configuration Chat Agent section lives here, not on the agents page", async ({
    testPage,
  }) => {
    // Regression guard for the move from /settings/agents to /settings/utility-agents.
    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Configuration Chat Agent", { exact: true })).toBeVisible();
    await expect(
      testPage.getByText(
        "Choose which agent profile to use for the Configuration Chat. This agent can manage your workflows, agent profiles, and MCP configuration.",
      ),
    ).toBeVisible();

    await testPage.goto("/settings/agents");
    await expect(testPage.getByRole("heading", { name: "Agents", exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByText("Configuration Chat Agent", { exact: true })).toHaveCount(0);
  });
});
