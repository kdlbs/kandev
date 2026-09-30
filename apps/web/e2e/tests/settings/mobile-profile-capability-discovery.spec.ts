import { test, expect } from "../../fixtures/test-base";
import {
  createProfileWithCatalog,
  mockProfileProbeRequiresAuth,
  observeProfileDiscoveryRequests,
  readProfileProbeEvidence,
} from "../../helpers/profile-capability-discovery";

test.describe("Mobile profile capability discovery", () => {
  test("refreshes a launch draft through touch controls and retains the latest options", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is required for profile discovery E2E");
    const { profile, evidencePath } = await createProfileWithCatalog(
      apiClient,
      backend,
      agent.id,
      "Mobile capability discovery",
      "mobile-saved",
    );
    const requests = observeProfileDiscoveryRequests(testPage);

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );

      await testPage.getByTestId("env-var-row-0").locator("input").nth(1).fill("mobile-draft");
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "stale",
      );
      const refresh = testPage.getByTestId("profile-refresh-capabilities");
      const refreshBounds = await refresh.boundingBox();
      expect(refreshBounds?.height).toBeGreaterThanOrEqual(44);
      await expect(
        testPage.getByRole("button", { name: /^Save( changes)?$/i }).first(),
      ).toBeEnabled();

      await testPage.getByTestId("cli-flag-flag-0").fill("--profile-catalog=mobile-draft-cli");
      await testPage.getByTestId("profile-advanced-options-trigger").tap();
      await testPage.getByTestId("command-prefix-input").fill("mock-agent --profile-probe-wrapper");
      await refresh.tap();
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );

      await selector.tap();
      await testPage.getByRole("option", { name: "Profile env mobile-draft" }).tap();
      await expect(selector).toContainText("Profile env mobile-draft");
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden({
        timeout: 15_000,
      });
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);

      const resolveRequest = requests.find(
        (request) =>
          request.url.endsWith("/resolve") &&
          request.body.model === "profile-env-mobile-draft" &&
          request.body.launch_settings !== undefined,
      );
      expect(resolveRequest?.body.profile_id).toBe(profile.id);
      const probeRequest = requests.find(
        (request) => request.url.endsWith("/probe") && request.body.refresh === true,
      );
      expect(probeRequest?.body.profile_id).toBe(profile.id);
      expect(probeRequest?.body.launch_settings).toEqual({
        env_vars: [
          { key: "MOCK_AGENT_PROFILE_CATALOG", value: "mobile-draft" },
          { key: "MOCK_AGENT_PROFILE_EVIDENCE_FILE", value: evidencePath },
        ],
        cli_flags: [
          {
            description: "Profile catalog fixture",
            flag: "--profile-catalog=mobile-draft-cli",
            enabled: true,
          },
        ],
        command_prefix: "mock-agent --profile-probe-wrapper",
      });
      const evidence = readProfileProbeEvidence(evidencePath);
      expect(evidence.some((item) => item.wrapper && item.command === "mock-agent")).toBe(true);
      expect(
        evidence.some(
          (item) =>
            !item.wrapper &&
            item.env_catalog === "mobile-draft" &&
            item.cli_catalog === "mobile-draft-cli",
        ),
      ).toBe(true);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("keeps authentication recovery available after a profile probe fails", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(60_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is required for profile discovery E2E");
    const { profile } = await createProfileWithCatalog(
      apiClient,
      backend,
      agent.id,
      "Mobile capability auth recovery",
      "mobile-auth",
    );

    try {
      const authProbeIntercepted = await mockProfileProbeRequiresAuth(testPage, profile.id);
      await testPage.route("**/api/v1/host-shell/start", (route) =>
        route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "host shell is stubbed in this test" }),
        }),
      );
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await expect.poll(authProbeIntercepted).toBe(true);
      await expect(testPage.getByTestId("profile-no-auth-panel")).toHaveAttribute(
        "data-status",
        "auth_required",
        { timeout: 20_000 },
      );

      const openTerminal = testPage.getByTestId("profile-no-auth-open-terminal");
      await expect(openTerminal).toBeVisible();
      const bounds = await openTerminal.boundingBox();
      expect(bounds?.height).toBeGreaterThanOrEqual(44);
      await openTerminal.tap();
      await expect(testPage.getByRole("dialog")).toBeVisible();
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
