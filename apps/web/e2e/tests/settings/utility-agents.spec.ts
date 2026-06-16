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
  test("action model dropdown keeps wheel scrolling inside the menu", async ({ testPage }) => {
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
        if (target instanceof Element && target.closest('[data-slot="select-content"]')) {
          testWindow.__utilitySelectWheelEvents = (testWindow.__utilitySelectWheelEvents ?? 0) + 1;
        }
      });
    });

    const actionRow = testPage
      .getByText("commit-message", { exact: true })
      .locator("xpath=ancestor::div[contains(@class, 'items-center')][1]");
    const actionSelect = actionRow.getByRole("combobox");
    await actionSelect.click();
    const selectContent = testPage.locator('[data-slot="select-content"]').first();
    await expect(selectContent).toBeVisible();

    await selectContent.dispatchEvent("wheel", {
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
    // host utility capability cache. In E2E (KANDEV_MOCK_AGENT=only) only
    // `mock-agent` is registered in the inference-agent registry, so the
    // Agent dropdown shows exactly one option: Mock. (Non-OK agents are no
    // longer filtered out — see #1269 — but in this profile no other agent
    // is registered to begin with.)
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

    // The default-model section has an Agent select (shadcn) and the shared
    // model/config selector button used by profile settings.
    const agentSelect = testPage
      .locator('div:has(> label:text-is("Agent"))')
      .first()
      .getByRole("combobox");
    const modelSelector = testPage.getByRole("button", {
      name: "Default utility model settings",
    });

    // Model selector starts disabled until an agent is picked.
    await expect(modelSelector).toBeDisabled();

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

    // Model selector is now enabled. Open the popover and verify both
    // probed models are listed.
    await expect(modelSelector).toBeEnabled();
    await modelSelector.click();
    const suggestions = testPage.getByLabel("Suggestions");
    await expect(suggestions.getByText("Mock Fast", { exact: true })).toBeVisible();
    await expect(suggestions.getByText("Mock Smart", { exact: true })).toBeVisible();

    // Search input is part of the shared selector — filtering narrows the list.
    await testPage.getByPlaceholder("Filter models...").fill("smart");
    await expect(suggestions.getByText("Mock Fast", { exact: true })).toHaveCount(0);

    // Pick Mock Smart and verify the trigger reflects the selection.
    await suggestions.getByText("Mock Smart", { exact: true }).click();
    await expect(modelSelector).toContainText("Mock Smart");

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });

  test("non-ok agent renders status note + Refresh re-probes", async ({ testPage }) => {
    // Regression guard for "claude shown in picker but no models, with no
    // explanation". The backend now surfaces probe status so the UI can
    // render an inline note + Refresh button instead of a silently-empty
    // Model picker. Stub both endpoints to drive the state machine.
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    let refreshCount = 0;
    await testPage.route("**/api/v1/utility/inference-agents", (route) => {
      if (route.request().method() !== "GET") {
        return route.fallback();
      }
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "stub-acp",
              name: "Stub ACP Agent",
              display_name: "Stub",
              status: refreshCount === 0 ? "auth_required" : "ok",
              status_message: refreshCount === 0 ? "please run `stub login`" : "",
              models:
                refreshCount === 0
                  ? []
                  : [{ id: "stub-fast", name: "Stub Fast", description: "", is_default: true }],
            },
          ],
        }),
      });
    });

    await testPage.route("**/api/v1/utility/inference-agents/stub-acp/refresh", (route) => {
      refreshCount += 1;
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: "stub-acp",
          name: "Stub ACP Agent",
          display_name: "Stub",
          status: "ok",
          models: [{ id: "stub-fast", name: "Stub Fast", description: "", is_default: true }],
        }),
      });
    });

    await testPage.goto("/settings/utility-agents");
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // Pick Stub from the Agent dropdown so the Default Model row binds
    // selectedAgent to the auth_required entry.
    const agentSelect = testPage
      .locator('div:has(> label:text-is("Agent"))')
      .first()
      .getByRole("combobox");
    await agentSelect.click();
    await testPage.getByRole("option", { name: "Stub", exact: true }).click();

    // Auth_required status copy renders, Refresh visible.
    const note = testPage.getByTestId("inference-agent-status-note").first();
    await expect(note).toBeVisible();
    await expect(note).toContainText("Sign in to Stub");

    const refresh = testPage.getByTestId("inference-agent-refresh").first();
    await expect(refresh).toBeVisible();
    await refresh.click();

    // After Refresh, status note disappears (status:"ok" + 1 model).
    await expect(testPage.getByTestId("inference-agent-status-note").first()).not.toBeVisible();
    expect(refreshCount).toBe(1);

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
