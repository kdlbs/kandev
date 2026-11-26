import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { installAntigravityRuntimeFixture } from "./antigravity-runtime-setup-helpers";

test.describe("Antigravity runtime setup on desktop", () => {
  test("uses CLI sign-in and refreshes its model catalog", async ({ testPage }) => {
    const fixture = await installAntigravityRuntimeFixture(testPage, "cli");
    await testPage.goto("/settings/agents");

    const card = testPage.getByTestId("agent-group-antigravity-acp");
    await expect(card).toContainText("Antigravity CLI");
    const authAction = card.getByTestId("auth-icon-antigravity-acp");
    const loginStarted = waitForHttp(
      testPage,
      "POST",
      /\/api\/v1\/agent-login\/agents\/antigravity-acp\/start$/,
    );
    await authAction.click();
    await loginStarted;

    await expect(testPage.getByRole("dialog")).toBeVisible();
    await expect(testPage.getByTestId("agent-login-command")).toContainText(
      "/opt/antigravity cli/agy acp --login",
    );
    expect(fixture.loginVariant()).toBe("antigravity-cli");

    const modelRefresh = waitForHttp(testPage, "GET", /\/api\/v1\/agent-models\/antigravity-acp$/);
    const readinessRefresh = waitForHttp(testPage, "GET", /\/api\/v1\/agents\/available$/);
    await testPage.getByTestId("agent-login-done").click();
    const [modelRefreshResponse] = await Promise.all([modelRefresh, readinessRefresh]);
    expect(new URL(modelRefreshResponse.url()).searchParams.get("refresh")).toBe("true");

    await expect(card.getByTestId("auth-icon-antigravity-acp")).toHaveCount(0);
    await expect(card).toContainText("Antigravity CLI");
    expect(fixture.modelRefreshes()).toBe(1);
  });

  test("labels the fallback install and links to Google CLI setup", async ({ testPage }) => {
    const available = {
      name: "antigravity-acp",
      display_name: "Antigravity",
      description: "Antigravity ACP support.",
      supports_local_install: true,
      supports_mcp: true,
      mcp_config_path: null,
      installation_paths: [],
      available: false,
      matched_path: null,
      capabilities: {
        supports_session_resume: true,
        supports_shell: false,
        supports_workspace_only: false,
      },
      model_config: {
        default_model: "",
        runtime_variant: "standalone",
        available_models: [],
        available_modes: [],
        supports_dynamic_models: false,
        status: "not_installed",
        error: "",
      },
      updated_at: "2026-11-24T00:00:00Z",
    };
    await testPage.route("**/api/v1/agents/available**", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ agents: [available], tools: [], total: 1 }),
      }),
    );
    await testPage.route("**/api/v1/agents/discovery**", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ agents: [], total: 0 }),
      }),
    );
    await testPage.goto("/settings/agents/browse");

    const card = testPage.getByTestId("install-card-antigravity-acp");
    await expect(card).toBeVisible();
    await expect(card.getByRole("button", { name: "Install standalone ACP server" })).toBeVisible();
    await expect(card.getByTestId("antigravity-cli-setup-guide")).toHaveAttribute(
      "href",
      "https://antigravity.google/docs/cli",
    );
  });
});
