import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import type { Locator } from "@playwright/test";
import { installAntigravityRuntimeFixture } from "./antigravity-runtime-setup-helpers";

test.describe("Antigravity runtime setup on phone", () => {
  test("Antigravity CLI permission flag saves by touch", async ({
    backend,
    testPage,
    apiClient,
    prCapture,
  }) => {
    const restoreBackend = await backend.useEnv({ KANDEV_E2E_MOCK_ANTIGRAVITY: "true" });
    let profileId: string | undefined;

    try {
      const { agents } = await apiClient.listAgents();
      const agent = agents.find((candidate) => candidate.name === "antigravity-acp");
      if (!agent) throw new Error("Antigravity agent is not available in the E2E registry");
      const profile = await apiClient.createAgentProfile(agent.id, "Mobile Antigravity CLI flag", {
        model: "",
        cli_flags: [],
      });
      profileId = profile.id;
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const flagSwitch = testPage.getByTestId(
        "cli-flag-curated-enabled-antigravity_skip_permissions",
      );
      await expect(flagSwitch).toBeVisible({ timeout: 15_000 });
      await expect(flagSwitch).toHaveAttribute("data-state", "unchecked");
      const box = await flagSwitch.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
      expect(box!.width).toBeGreaterThanOrEqual(44);
      await flagSwitch.tap();
      await expect(flagSwitch).toHaveAttribute("data-state", "checked");

      const saveButton = testPage
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: /^Save( changes)?$/i });
      await expect(saveButton).toHaveCount(1);
      await saveButton.tap();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await expect
        .poll(() => apiClient.getAgentProfile(profile.id))
        .toMatchObject({
          cliFlags: [
            expect.objectContaining({ flag: "--dangerously-skip-permissions", enabled: true }),
          ],
        });

      await testPage.reload();
      await expect(flagSwitch).toHaveAttribute("data-state", "checked", { timeout: 15_000 });
      await flagSwitch.scrollIntoViewIfNeeded();
      await prCapture.screenshot("antigravity-cli-flag-phone", {
        caption: "Antigravity CLI permission switch on the phone profile page",
      });
      await flagSwitch.tap();
      await expect(saveButton).toHaveCount(1);
      await saveButton.tap();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
      await testPage.reload();
      await expect(flagSwitch).toHaveAttribute("data-state", "unchecked", { timeout: 15_000 });
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId).catch(() => {});
      await restoreBackend();
    }
  });

  test("opens standalone consent, restores focus, and refreshes readiness", async ({
    testPage,
  }) => {
    const fixture = await installAntigravityRuntimeFixture(testPage, "standalone");
    await testPage.goto("/settings/agents");

    const card = testPage.getByTestId("agent-group-antigravity-acp");
    await expect(card).toContainText("Standalone ACP server");
    const authAction = card.getByTestId("auth-icon-antigravity-acp");
    const actionBox = await authAction.boundingBox();
    expect(actionBox).not.toBeNull();
    expect(actionBox!.height).toBeGreaterThanOrEqual(44);
    expect(actionBox!.width).toBeGreaterThanOrEqual(44);

    await authAction.tap();
    const sheet = testPage.getByTestId("antigravity-login-sheet");
    await expect(sheet).toBeVisible();
    await expect(sheet).toContainText("standalone Antigravity ACP server");
    await expect(testPage.getByTestId("agent-login-dialog")).toHaveCount(0);
    const close = sheet.getByRole("button", { name: "Close", exact: true });
    const viewport = testPage.viewportSize()!;
    await expectInViewport(close, viewport);
    const closeBox = await close.boundingBox();
    expect(closeBox).not.toBeNull();
    expect(closeBox!.height).toBeGreaterThanOrEqual(44);
    await close.tap();
    await expect(sheet).toBeHidden();
    await expect(authAction).toBeFocused();

    await authAction.tap();
    const reopenedSheet = testPage.getByTestId("antigravity-login-sheet");
    await expect(reopenedSheet).toBeVisible();
    const consentStart = waitForHttp(testPage, "POST", /\/api\/v1\/antigravity-acp\/login$/);
    const consentPoll = waitForHttp(
      testPage,
      "GET",
      /\/api\/v1\/antigravity-acp\/login\/antigravity-e2e-consent$/,
    );
    const readinessRefresh = waitForHttp(testPage, "GET", /\/api\/v1\/agents\/available$/);
    const start = reopenedSheet.getByRole("button", { name: "Sign in with Google" });
    const startBox = await start.boundingBox();
    expect(startBox).not.toBeNull();
    expect(startBox!.height).toBeGreaterThanOrEqual(44);
    await start.tap();
    await Promise.all([consentStart, consentPoll, readinessRefresh]);

    await expect(card.getByTestId("auth-icon-antigravity-acp")).toHaveCount(0);
    await expect(card).toContainText("Standalone ACP server");
    expect(fixture.consentPolls()).toBeGreaterThan(0);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    await expectInViewport(sheet, viewport);
  });

  test("keeps the CLI guide and standalone install action touch-sized", async ({ testPage }) => {
    await testPage.route("**/api/v1/agents/available**", (route) =>
      route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              name: "antigravity-acp",
              display_name: "Antigravity",
              supports_local_install: true,
              supports_mcp: true,
              installation_paths: [],
              available: false,
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
              },
              updated_at: "2026-11-24T00:00:00Z",
            },
          ],
          tools: [],
          total: 1,
        }),
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
    const install = card.getByRole("button", { name: "Install standalone ACP server" });
    await expect(install).toBeVisible();
    expect((await install.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await expect(card.getByTestId("antigravity-cli-setup-guide")).toBeVisible();
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});

async function expectInViewport(locator: Locator, viewport: { width: number; height: number }) {
  await expect
    .poll(async () => {
      const box = await locator.boundingBox();
      return Boolean(
        box &&
        box.x >= 0 &&
        box.y >= 0 &&
        box.x + box.width <= viewport.width &&
        box.y + box.height <= viewport.height,
      );
    })
    .toBe(true);
}
