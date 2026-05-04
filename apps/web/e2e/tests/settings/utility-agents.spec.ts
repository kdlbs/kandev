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
      testPage.getByText("One-shot AI helpers for commits, PRs, and prompts."),
    ).toBeVisible();

    // Default-model section.
    await expect(
      testPage.getByRole("heading", { name: "Default utility agent model", exact: true }),
    ).toBeVisible();

    // Built-in actions (seeded on first boot — see builtins.go).
    // Assert a representative subset; the full list lives server-side.
    await expect(testPage.getByText("commit-message", { exact: true })).toBeVisible();
    await expect(testPage.getByText("pr-title", { exact: true })).toBeVisible();
    await expect(testPage.getByText("enhance-prompt", { exact: true })).toBeVisible();

    // Custom agents section + empty state.
    await expect(
      testPage.getByRole("heading", { name: "Custom utility agents", exact: true }),
    ).toBeVisible();
    await expect(testPage.getByText("No custom utility agents.")).toBeVisible();

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
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

  test("selecting an agent populates the model combobox (ACP probe)", async ({
    testPage,
    backend,
  }) => {
    // Regression guard for "I select an agent but can't select a model".
    // The mock-agent binary advertises `mock-fast` (default) and `mock-smart`
    // in its session/new response, so the boot-time ACP probe populates the
    // host utility capability cache. The backend filters out agents whose
    // probe didn't reach StatusOK, so in E2E (KANDEV_MOCK_AGENT=only) the
    // Agent dropdown should show exactly one option: Mock.
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    // The probe runs in a goroutine at boot, so the first page load may
    // land before the cache is populated. Poll the backend directly until
    // mock-agent is reported with its models so the UI assertions below
    // aren't racing the probe.
    await expect
      .poll(
        async () => {
          const resp = await testPage.request.get(
            `${backend.baseUrl}/api/v1/utility/inference-agents`,
          );
          if (!resp.ok()) return 0;
          const data = (await resp.json()) as {
            agents: { id: string; models?: { id: string }[] | null }[];
          };
          const mock = data.agents.find((a) => a.id === "mock-agent");
          return mock?.models?.length ?? 0;
        },
        { timeout: 15_000, intervals: [250, 500, 1000] },
      )
      .toBeGreaterThanOrEqual(2);

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // The default-model section has an Agent select (shadcn) and a Model
    // combobox (the shared ModelCombobox from the profile page). Each is
    // scoped by the Label above it (no `htmlFor`).
    const agentSelect = testPage
      .locator('div:has(> label:text-is("Agent"))')
      .first()
      .getByRole("combobox");
    const modelCombobox = testPage
      .locator('div:has(> label:text-is("Model"))')
      .first()
      .getByRole("combobox");

    // Model combobox starts disabled until an agent is picked.
    await expect(modelCombobox).toBeDisabled();

    // Open the Agent dropdown: the only healthy option in E2E is Mock.
    // This implicitly guards the backend filter — if an auth_required or
    // still-probing agent had leaked through, it would show up here too.
    await agentSelect.click();
    const agentListbox = testPage.getByRole("listbox");
    await expect(agentListbox).toBeVisible();
    await expect(agentListbox.getByRole("option")).toHaveCount(1);
    await expect(agentListbox.getByRole("option", { name: "Mock", exact: true })).toBeVisible();
    await agentListbox.getByRole("option", { name: "Mock", exact: true }).click();
    await expect(agentListbox).not.toBeVisible();

    // Model combobox is now enabled. Open the popover and verify both
    // probed models are listed as command items, along with the "(default)"
    // badge on mock-fast.
    await expect(modelCombobox).toBeEnabled();
    await modelCombobox.click();
    await expect(testPage.getByRole("option", { name: /Mock Fast.*\(default\)/ })).toBeVisible();
    await expect(testPage.getByRole("option", { name: /Mock Smart/ })).toBeVisible();

    // Search input is part of the ModelCombobox — filtering narrows the list.
    await testPage.getByPlaceholder("Search models...").fill("smart");
    await expect(testPage.getByRole("option", { name: /Mock Fast/ })).toHaveCount(0);

    // Pick Mock Smart and verify the trigger reflects the selection.
    await testPage.getByRole("option", { name: /Mock Smart/ }).click();
    await expect(modelCombobox).toContainText("Mock Smart");

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
    await expect(
      testPage.getByRole("heading", { name: "Configuration Chat Agent", exact: true }),
    ).toBeVisible();
    await expect(
      testPage.getByText(
        "Choose which agent profile to use for the Configuration Chat. This agent can manage your workflows, agent profiles, and MCP configuration.",
      ),
    ).toBeVisible();

    await testPage.goto("/settings/agents");
    await expect(testPage.getByRole("heading", { name: "Agents", exact: true })).toBeVisible({
      timeout: 15_000,
    });
    await expect(
      testPage.getByRole("heading", { name: "Configuration Chat Agent", exact: true }),
    ).toHaveCount(0);
  });
});
