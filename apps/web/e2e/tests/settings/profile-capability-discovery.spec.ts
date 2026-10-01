import { test, expect } from "../../fixtures/test-base";
import {
  createProfileWithCatalog,
  observeProfileDiscoveryRequests,
  readProfileProbeEvidence,
} from "../../helpers/profile-capability-discovery";

test.describe("Profile capability discovery", () => {
  test("isolates saved catalogs and refreshes an unsaved launch draft before save", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is required for profile discovery E2E");

    const alpha = await createProfileWithCatalog(
      apiClient,
      backend,
      agent.id,
      "Capability discovery Alpha",
      "alpha",
    );
    const beta = await createProfileWithCatalog(
      apiClient,
      backend,
      agent.id,
      "Capability discovery Beta",
      "beta",
    );
    const requests = observeProfileDiscoveryRequests(testPage);

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${alpha.profile.id}`);
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );
      await selector.click();
      await expect(testPage.getByRole("option", { name: "Profile env alpha" })).toBeVisible();
      await expect(testPage.getByRole("option", { name: "Profile CLI alpha" })).toBeVisible();
      await testPage.keyboard.press("Escape");

      await testPage.goto(`/settings/agents/${agent.name}/profiles/${beta.profile.id}`);
      const betaSelector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(betaSelector).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );
      await betaSelector.click();
      await expect(testPage.getByRole("option", { name: "Profile env beta" })).toBeVisible();
      await expect(testPage.getByRole("option", { name: "Profile env alpha" })).toHaveCount(0);
      await testPage.keyboard.press("Escape");

      await testPage.goto(`/settings/agents/${agent.name}/profiles/${alpha.profile.id}`);
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );

      const catalogValue = testPage.getByTestId("env-var-row-0").locator("input").nth(1);
      await catalogValue.fill("draft-env");
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "stale",
      );
      const saveButton = testPage.getByRole("button", { name: /^Save( changes)?$/i }).first();
      await expect(saveButton).toBeEnabled();

      await testPage.getByTestId("cli-flag-flag-0").fill("--profile-catalog=draft-cli");
      await testPage.getByTestId("profile-advanced-options-trigger").click();
      await testPage.getByTestId("command-prefix-input").fill("mock-agent --profile-probe-wrapper");
      await testPage.getByTestId("profile-refresh-capabilities").click();
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );

      await selector.click();
      const draftModel = testPage.getByRole("option", { name: "Profile env draft-env" });
      await expect(draftModel).toBeVisible();
      await testPage.getByRole("option", { name: "Profile CLI draft-cli" }).click();
      await expect(selector).toContainText("Profile CLI draft-cli");
      await expect
        .poll(() =>
          requests.some(
            (request) =>
              request.url.endsWith("/resolve") && request.body.model === "profile-cli-draft-cli",
          ),
        )
        .toBe(true);
      const selectedModelRequest = requests.find(
        (request) =>
          request.url.endsWith("/resolve") && request.body.model === "profile-cli-draft-cli",
      );
      try {
        await expect
          .poll(
            () => selectedModelRequest?.responseStatus ?? selectedModelRequest?.failure ?? null,
            { timeout: 20_000 },
          )
          .toBe(200);
      } catch {
        throw new Error(
          `Profile model-options probe did not return: ${JSON.stringify(selectedModelRequest)}`,
        );
      }
      await expect(testPage.getByTestId("model-config-resolution-loading")).toBeHidden({
        timeout: 15_000,
      });
      expect(
        requests.filter(
          (request) =>
            request.url.endsWith("/resolve") && request.body.model === "profile-cli-draft-cli",
        ),
      ).toHaveLength(1);
      await expect(saveButton).toBeEnabled({ timeout: 10_000 });
      await saveButton.click();
      await expect(testPage.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });

      const draftProbe = requests.find(
        (request) => request.url.endsWith("/probe") && request.body.refresh === true,
      );
      expect(draftProbe?.body.profile_id).toBe(alpha.profile.id);
      expect(draftProbe?.body.launch_settings).toEqual({
        env_vars: [
          { key: "MOCK_AGENT_PROFILE_CATALOG", value: "draft-env" },
          { key: "MOCK_AGENT_PROFILE_EVIDENCE_FILE", value: alpha.evidencePath },
        ],
        cli_flags: [
          {
            description: "Profile catalog fixture",
            flag: "--profile-catalog=draft-cli",
            enabled: true,
          },
        ],
        command_prefix: "mock-agent --profile-probe-wrapper",
      });
      expect(
        requests.some(
          (request) =>
            request.url.endsWith("/probe") &&
            request.body.profile_id === alpha.profile.id &&
            request.body.launch_settings === undefined,
        ),
      ).toBe(true);
      expect(
        requests.some(
          (request) =>
            request.url.endsWith("/resolve") &&
            request.body.model === "profile-cli-draft-cli" &&
            request.body.profile_id === alpha.profile.id &&
            request.body.launch_settings !== undefined,
        ),
      ).toBe(true);

      const stored = await apiClient.getAgentProfile(alpha.profile.id);
      expect(stored.model).toBe("profile-cli-draft-cli");
      expect(stored.envVars?.find((item) => item.key === "MOCK_AGENT_PROFILE_CATALOG")?.value).toBe(
        "draft-env",
      );

      await testPage.reload();
      await expect(selector).toContainText("Profile CLI draft-cli", { timeout: 20_000 });

      const evidence = readProfileProbeEvidence(alpha.evidencePath);
      expect(evidence.some((item) => item.wrapper && item.command === "mock-agent")).toBe(true);
      expect(
        evidence.some(
          (item) =>
            !item.wrapper &&
            item.env_catalog === "draft-env" &&
            item.cli_catalog === "draft-cli" &&
            item.profile_args?.[0] === "--profile-catalog=draft-cli",
        ),
      ).toBe(true);
      expect(
        readProfileProbeEvidence(beta.evidencePath).some((item) => item.env_catalog === "beta"),
      ).toBe(true);
    } finally {
      await apiClient.deleteAgentProfile(alpha.profile.id, true);
      await apiClient.deleteAgentProfile(beta.profile.id, true);
    }
  });
});
