import { injectLatency } from "../../helpers/causal-waits";
import { test, expect } from "../../fixtures/test-base";
import {
  mockProfileProbeRequiresAuth,
  observeProfileDiscoveryRequests,
} from "../../helpers/profile-capability-discovery";

test.describe("Onboarding agent setup", () => {
  // @covers AC-AGENTS-FIRST-RUN-SETUP-001.5
  test("bounds a long agent catalog on tall and short viewports while navigation stays fixed", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 1280, height: 1200 });
    await testPage.route("**/api/v1/agents/available", async (route) => {
      const response = await route.fetch();
      const payload = await response.json();
      const mock = payload.agents.find((agent: { name: string }) => agent.name === "mock-agent");
      expect(mock).toBeTruthy();
      await route.fulfill({
        response,
        json: {
          ...payload,
          agents: [
            mock,
            ...Array.from({ length: 20 }, (_, i) => ({
              ...mock,
              name: `catalog-agent-${i}`,
              display_name: `Catalog agent ${i + 1}`,
            })),
          ],
        },
      });
    });
    await testPage.addInitScript(() => localStorage.removeItem("kandev.onboarding.completed"));
    await testPage.goto("/");
    const dialog = testPage.getByRole("dialog");
    await expect(dialog.getByRole("button", { name: /Catalog agent 20/ })).toHaveCount(1);
    await expect.poll(async () => (await dialog.boundingBox())?.height).toBeLessThanOrEqual(720);
    const body = dialog.getByTestId("onboarding-agent-body");
    const heading = dialog.getByRole("heading", { name: "AI Agents" });
    const next = dialog.getByRole("button", { name: "Next" });
    const before = { heading: await heading.boundingBox(), next: await next.boundingBox() };
    expect(await body.evaluate((el) => el.scrollHeight > el.clientHeight)).toBe(true);
    await body.evaluate((el) => {
      el.scrollTop = el.scrollHeight;
    });
    await expect.poll(() => body.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
    expect(await heading.boundingBox()).toEqual(before.heading);
    expect(await next.boundingBox()).toEqual(before.next);
    await testPage.setViewportSize({ width: 1280, height: 500 });
    await expect
      .poll(async () => {
        const bounds = await dialog.boundingBox();
        return bounds && bounds.y >= 15 && bounds.y + bounds.height <= 485;
      })
      .toBe(true);
    await next.click();
    await expect(dialog.getByRole("heading", { name: "Executors" })).toBeVisible();
  });

  // @covers AC-AGENTS-FIRST-RUN-SETUP-002.4, AC-AGENTS-FIRST-RUN-SETUP-003.1
  test("shares model options with profiles and preserves them when reopening the provider", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const profile = agents.find((item) => item.name === "mock-agent")?.profiles?.[0];
    if (!profile) throw new Error("Saved mock profile required for model options");
    await apiClient.updateAgentProfile(profile.id, {
      model: "mock-fast",
      config_options: { effort: "medium" },
    });
    const before = await apiClient.getAgentProfile(profile.id);
    const requests = observeProfileDiscoveryRequests(testPage);
    try {
      await testPage.addInitScript(() => localStorage.removeItem("kandev.onboarding.completed"));
      await testPage.goto("/");
      const trigger = testPage.getByRole("button", { name: /^Mock / });
      await trigger.click();
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
      await expect(selector).toContainText("Medium");
      const probes = requests.filter((request) => request.url.endsWith("/probe")).length;
      const resolves = requests.filter((request) => request.url.endsWith("/resolve")).length;
      await trigger.click();
      await trigger.click();
      await expect(selector).toBeEnabled();
      await selector.click();
      await testPage.getByTestId("config-option-trigger-effort").click();
      await testPage.getByRole("button", { name: "High", exact: true }).click();
      await expect(selector).toContainText("High");
      expect(requests.filter((request) => request.url.endsWith("/probe"))).toHaveLength(probes);
      // A deliberate option change may resolve dependent choices; reopening does not.
      expect(
        requests.filter((request) => request.url.endsWith("/resolve")).length,
      ).toBeGreaterThanOrEqual(resolves);
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
      await selector.click();
      const save = testPage.waitForRequest(
        (request) =>
          request.method() === "PATCH" && request.url().endsWith(`/agent-profiles/${profile.id}`),
      );
      await testPage.getByRole("button", { name: "Next" }).click();
      expect((await save).postDataJSON()).toEqual({ config_options: { effort: "high" } });
      await expect(testPage.getByRole("heading", { name: "Executors" })).toBeVisible();
      const after = await apiClient.getAgentProfile(profile.id);
      expect(after.configOptions).toEqual({ effort: "high" });
      expect(after.model).toBe(before.model);
      expect(after.autoApprove).toBe(before.autoApprove);
      expect(after.cliPassthrough).toBe(before.cliPassthrough);
    } finally {
      await apiClient.updateAgentProfile(profile.id, {
        model: profile.model,
        config_options: profile.configOptions ?? {},
      });
    }
  });

  test("waits for dependent options before saving a newly selected model", async ({
    testPage,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const profile = agents.find((item) => item.name === "mock-agent")?.profiles?.[0];
    if (!profile) throw new Error("Saved mock profile required");
    await apiClient.updateAgentProfile(profile.id, { model: "mock-fast" });
    await testPage.route("**/api/v1/agent-models/mock-agent/resolve", async (route) => {
      if (route.request().postDataJSON().model === "mock-smart")
        await injectLatency(
          1500,
          "exercises the onboarding save interlock during dependent model-option resolution",
        );
      await route.fallback();
    });
    try {
      await testPage.addInitScript(() => localStorage.removeItem("kandev.onboarding.completed"));
      await testPage.goto("/");
      await testPage.getByRole("button", { name: /^Mock / }).click();
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toContainText("Mock Fast");
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
      await selector.click();
      const resolved = testPage.waitForResponse(
        (response) =>
          response.url().endsWith("/agent-models/mock-agent/resolve") &&
          response.request().postDataJSON().model === "mock-smart",
      );
      await testPage.getByRole("option", { name: "Mock Smart", exact: true }).click();
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeVisible();
      const next = testPage.getByRole("button", { name: "Next" });
      await expect(next).toBeDisabled();
      await expect(testPage.getByRole("button", { name: "Skip" })).toBeEnabled();
      await resolved;
      await expect(next).toBeEnabled();
      await selector.click();
      await next.click();
      await expect(testPage.getByRole("heading", { name: "Executors" })).toBeVisible();
      expect((await apiClient.getAgentProfile(profile.id)).model).toBe("mock-smart");
    } finally {
      await apiClient.updateAgentProfile(profile.id, {
        model: profile.model,
        config_options: profile.configOptions ?? {},
      });
    }
  });

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

  test("discovers saved profile models automatically and saves only edited setup fields", async ({
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
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
      if ((await selector.getAttribute("aria-expanded")) === "true") await selector.click();
      await testPage.getByTestId("onboarding-agent-passthrough-switch").click();
      expect(
        requests
          .filter((request) => request.url.endsWith("/probe"))
          .every((request) => request.body.refresh !== true),
      ).toBe(true);

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
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden();
      if ((await selector.getAttribute("aria-expanded")) === "true") await selector.click();
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
