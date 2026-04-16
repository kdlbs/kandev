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

  test("selecting an agent populates the model dropdown (ACP probe)", async ({
    testPage,
    backend,
  }) => {
    // Regression guard for "I select an agent but can't select a model".
    // Models are populated from the host utility capability cache, which is
    // seeded by the boot-time ACP probe. The mock-agent binary advertises
    // `mock-fast` (default) and `mock-smart` in its session/new response, so
    // after the probe completes the page must show those two in the Model
    // dropdown once the user picks Mock as the agent.
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    await testPage.goto("/settings/utility-agents");

    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // The default-model section has two Selects side by side: Agent | Model.
    // Each is scoped by the Label above it (no `htmlFor`, so we locate via
    // the containing div).
    const agentSelect = testPage
      .locator('div:has(> label:text-is("Agent"))')
      .first()
      .getByRole("combobox");
    const modelSelect = testPage
      .locator('div:has(> label:text-is("Model"))')
      .first()
      .getByRole("combobox");

    // Model dropdown starts disabled until an agent is picked — guards that
    // the UI enforces "agent first" ordering.
    await expect(modelSelect).toBeDisabled();

    // The probe runs in a goroutine at boot, so the agent-list fetch from
    // SSR/client may land before probe models are cached. Poll the backend
    // directly until the inference-agents response carries models for
    // mock-agent so the assertions below aren't racing the probe.
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

    // Re-fetch the page so the initial state picks up the now-populated
    // models (the section reads them from its own load snapshot, not live).
    await testPage.reload();
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // Open the Agent dropdown and pick Mock.
    await agentSelect.click();
    const listbox = testPage.getByRole("listbox");
    await expect(listbox).toBeVisible();
    await expect(listbox.getByRole("option", { name: "Mock", exact: true })).toBeVisible();
    await listbox.getByRole("option", { name: "Mock", exact: true }).click();
    await expect(listbox).not.toBeVisible();

    // The model dropdown should now be enabled and carry both probed models.
    await expect(modelSelect).toBeEnabled();
    await modelSelect.click();
    const modelListbox = testPage.getByRole("listbox");
    await expect(modelListbox).toBeVisible();
    await expect(
      modelListbox.getByRole("option", { name: "Mock Fast", exact: true }),
    ).toBeVisible();
    await expect(
      modelListbox.getByRole("option", { name: "Mock Smart", exact: true }),
    ).toBeVisible();

    // Pick a non-default model and verify it's reflected back on the trigger.
    await modelListbox.getByRole("option", { name: "Mock Smart", exact: true }).click();
    await expect(modelListbox).not.toBeVisible();
    await expect(modelSelect).toContainText("Mock Smart");

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });
});
