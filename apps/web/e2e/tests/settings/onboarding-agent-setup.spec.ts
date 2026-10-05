import { test, expect } from "../../fixtures/test-base";
import {
  mockProfileProbeRequiresAuth,
  observeProfileDiscoveryRequests,
} from "../../helpers/profile-capability-discovery";

test.describe("Onboarding agent setup", () => {
  test("opens the saved profile for authentication recovery without saving a tour draft", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    const profile = agent?.profiles?.[0];
    if (!agent || !profile)
      throw new Error("Saved mock profile is required for authentication recovery");
    const before = await apiClient.getAgentProfile(profile.id);
    const writes: string[] = [];
    testPage.on("request", (request) => {
      if (request.method() === "PATCH" && request.url().endsWith(`/agent-profiles/${profile.id}`)) {
        writes.push(request.url());
      }
    });
    await mockProfileProbeRequiresAuth(testPage, profile.id);
    await testPage.addInitScript(() => localStorage.removeItem("kandev.onboarding.completed"));
    await testPage.goto("/");
    await testPage.getByRole("button", { name: /^Mock / }).click();
    const fields = testPage.getByTestId("onboarding-agent-setup-fields");
    await expect(fields.getByRole("alert")).toContainText(/authentication.*required/i);
    await expect(
      fields.getByRole("button", { name: "Profile start model settings" }),
    ).toBeDisabled();
    const recovery = fields.getByRole("link", { name: "Settings" });
    await expect(recovery).toHaveAttribute(
      "href",
      `/settings/agents/${agent.name}/profiles/${profile.id}`,
    );
    await recovery.click();
    await expect(testPage.getByTestId("profile-no-auth-panel")).toBeVisible();
    await expect(testPage.getByRole("dialog")).toHaveCount(0);
    expect(writes).toEqual([]);
    const after = await apiClient.getAgentProfile(profile.id);
    expect(after.model).toBe(before.model);
    expect(after.cliPassthrough).toBe(before.cliPassthrough);
    expect(
      await testPage.evaluate(() => localStorage.getItem("kandev.onboarding.completed")),
    ).toBeNull();
  });

  test("probes saved profile context automatically, exposes only model and passthrough, and saves exact partial patch", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);

    const { agents } = await apiClient.listAgents();
    const mockAgent = agents.find((a) => a.name === "mock-agent");
    if (!mockAgent) throw new Error("mock-agent required for onboarding agent setup E2E");

    const profile = mockAgent.profiles?.[0];
    if (!profile) throw new Error("saved mock profile required for onboarding setup");
    await apiClient.updateAgentProfile(profile.id, { model: "mock-fast", cli_passthrough: false });
    const before = await apiClient.getAgentProfile(profile.id);
    const requests = observeProfileDiscoveryRequests(testPage);
    try {
      // Open first-run dialog by removing completion marker
      await testPage.addInitScript(() => {
        localStorage.removeItem("kandev.onboarding.completed");
      });

      await testPage.goto("/");
      const dialog = testPage.getByRole("dialog");
      await expect(dialog).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByRole("heading", { name: "AI Agents" })).toBeVisible();

      // Expand mock agent
      const mockRowTrigger = testPage.getByRole("button", { name: /^Mock / });
      await expect(mockRowTrigger).toBeVisible();
      await mockRowTrigger.click();

      // Verify dedicated setup fields are rendered
      const setupFields = testPage.getByTestId("onboarding-agent-setup-fields");
      await expect(setupFields).toBeVisible();

      // Model selector is present
      const modelField = testPage.getByTestId("onboarding-agent-model-field");
      await expect(modelField).toBeVisible();

      await expect
        .poll(() =>
          requests.some(
            (request) =>
              request.url.endsWith("/probe") &&
              request.body.profile_id === profile.id &&
              request.body.launch_settings === undefined &&
              request.responseStatus === 200,
          ),
        )
        .toBe(true);
      const selector = modelField.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeEnabled();
      await selector.click();
      await testPage.getByRole("option", { name: "Mock Smart", exact: true }).click();
      await expect(selector).toContainText("Mock Smart");
      await testPage.getByTestId("onboarding-agent-passthrough-switch").click();
      expect(requests.filter((request) => request.url.endsWith("/resolve"))).toHaveLength(0);

      // Refresh icon button is present
      const refreshBtn = testPage.getByTestId("onboarding-agent-refresh-models");
      await expect(refreshBtn).toBeVisible();
      const refreshBounds = await refreshBtn.boundingBox();
      expect(refreshBounds).not.toBeNull();
      expect(refreshBounds!.width).toBeCloseTo(28, 0);
      expect(refreshBounds!.height).toBeCloseTo(28, 0);

      // Verify hidden settings are NOT mounted
      await expect(testPage.getByText(/Auto-approve/i)).toHaveCount(0);
      await expect(testPage.getByText(/Advanced settings/i)).toHaveCount(0);
      await expect(testPage.getByText(/Fallback settings/i)).toHaveCount(0);
      await expect(testPage.getByText(/CLI Flags/i)).toHaveCount(0);
      await expect(testPage.getByText(/Command prefix/i)).toHaveCount(0);

      // Verify help text and auto-approve warning
      await expect(
        testPage.getByText(/More settings are available in Settings > Agents/i),
      ).toBeVisible();
      await expect(
        testPage.getByText(/The default Agent Profiles run with Auto Approve enabled/i),
      ).toBeVisible();

      // Proceed to next step
      const nextBtn = testPage.getByRole("button", { name: "Next" });
      await expect(nextBtn).toBeEnabled();
      const saveRequest = testPage.waitForRequest(
        (request) =>
          request.method() === "PATCH" && request.url().endsWith(`/agent-profiles/${profile.id}`),
      );
      await nextBtn.click();
      expect((await saveRequest).postDataJSON()).toEqual({
        model: "mock-smart",
        cli_passthrough: true,
      });

      // Verify step 1 (Executors) is shown
      await expect(testPage.getByRole("heading", { name: "Executors" })).toBeVisible();
      const saved = await apiClient.getAgentProfile(profile.id);
      expect(saved.model).toBe("mock-smart");
      expect(saved.cliPassthrough).toBe(true);
      for (const key of [
        "mode",
        "autoApprove",
        "cliFlags",
        "envVars",
        "commandPrefix",
        "configOptions",
        "fallbackModel",
        "autoFallback",
      ] as const) {
        expect(saved[key]).toEqual(before[key]);
      }

      await testPage.getByRole("button", { name: "Back", exact: true }).click();
      await mockRowTrigger.click();
      await selector.click();
      await testPage.getByRole("option", { name: "Mock Fast", exact: true }).click();
      await nextBtn.click();
      await expect(testPage.getByRole("heading", { name: "Executors" })).toBeVisible();
      expect((await apiClient.getAgentProfile(profile.id)).model).toBe("mock-fast");
    } finally {
      await apiClient.updateAgentProfile(profile.id, {
        model: profile.model,
        cli_passthrough: profile.cliPassthrough,
      });
    }
  });
});
