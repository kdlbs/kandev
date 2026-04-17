import { test, expect } from "../../fixtures/test-base";

/**
 * Regression guard for the onboarding "AI Agents" step. Specifically covers
 * the collapsed agent row surfacing a tooltip with the probe error message
 * on failed/auth-required states — previously the error was only visible
 * after expanding the row.
 */
test.describe("Onboarding: AI Agents step", () => {
  test("shows tooltip with error message on failed agent row", async ({ testPage }) => {
    // Stub the available-agents response with three synthetic rows — one ok,
    // one probing, one failed — so we don't depend on host-side probe timing.
    await testPage.route("**/api/v1/agents/available", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            buildAgent({ name: "ok-agent", display: "Ok Agent", status: "ok" }),
            buildAgent({ name: "probing-agent", display: "Probing Agent", status: "probing" }),
            buildAgent({
              name: "failed-agent",
              display: "Failed Agent",
              status: "failed",
              error: "probe timed out after 45s",
            }),
          ],
          tools: [],
          total: 3,
        }),
      }),
    );

    // The shared fixture pre-sets kandev.onboarding.completed=true; undo that
    // so the onboarding dialog renders.
    await testPage.addInitScript(() => {
      localStorage.removeItem("kandev.onboarding.completed");
    });

    await testPage.goto("/");

    await expect(testPage.getByRole("heading", { name: "AI Agents", exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // All three agent rows are listed as installed (filtered by agent.available).
    await expect(testPage.getByText("Ok Agent")).toBeVisible();
    await expect(testPage.getByText("Probing Agent")).toBeVisible();
    await expect(testPage.getByText("Failed Agent")).toBeVisible();

    // Ok row shows "Installed" with no error tooltip trigger.
    await expect(testPage.getByText("Installed", { exact: true })).toBeVisible();

    // Probing row shows the spinner + label.
    await expect(testPage.getByText("Probing", { exact: true })).toBeVisible();

    // Failed row shows "Error". Hovering reveals a tooltip with the message.
    const errorLabel = testPage.getByText("Error", { exact: true });
    await expect(errorLabel).toBeVisible();
    await errorLabel.hover();
    await expect(testPage.getByRole("tooltip")).toContainText("probe timed out after 45s");
  });
});

function buildAgent(opts: {
  name: string;
  display: string;
  status: "ok" | "probing" | "failed";
  error?: string;
}) {
  return {
    name: opts.name,
    display_name: opts.display,
    description: "",
    install_script: "",
    supports_mcp: false,
    mcp_config_path: null,
    installation_paths: [],
    available: true,
    matched_path: null,
    capabilities: {
      supports_session_resume: false,
      supports_shell: false,
      supports_workspace_only: false,
    },
    model_config: {
      default_model: "synthetic",
      available_models: [{ id: "synthetic", name: "synthetic" }],
      supports_dynamic_models: true,
      status: opts.status,
      error: opts.error,
    },
    permission_settings: {},
    passthrough_config: null,
    updated_at: new Date().toISOString(),
  };
}
