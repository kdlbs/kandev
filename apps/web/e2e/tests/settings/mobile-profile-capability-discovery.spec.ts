import { test, expect } from "../../fixtures/test-base";
import type { BrowserContext, Page } from "@playwright/test";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import {
  approveRuntimeUpdateInOtherTab,
  createProfileWithCatalog,
  installProfileRuntimeObservationFixture,
  mockProfileProbeRequiresAuth,
  observeProfileDiscoveryRequests,
  readProfileProbeEvidence,
} from "../../helpers/profile-capability-discovery";

async function expectMobileModelSettings(page: Page, capture: PrAssetCapture) {
  await expect(page.getByTestId("profile-runtime-info")).toHaveCount(0);
  const refresh = page.getByTestId("profile-refresh-capabilities");
  await refresh.scrollIntoViewIfNeeded();
  expect((await refresh.boundingBox())?.height).toBeGreaterThanOrEqual(44);
  await capture.screenshot("mobile-profile-model-settings", {
    caption: "Mobile profile model settings without runtime details",
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

async function editRuntimeDraft(page: Page): Promise<string> {
  const selector = page.getByRole("button", { name: "Profile start model settings" });
  const selectedBefore = (await selector.textContent()) ?? "";
  await page.getByTestId("env-var-row-0").locator("input").nth(1).fill("mobile-runtime-draft");
  await expect(page.getByTestId("profile-capability-status")).toHaveAttribute(
    "data-status",
    "stale",
  );
  return selectedBefore;
}

async function expectActivatedProfileDraft(
  page: Page,
  requests: ReturnType<typeof observeProfileDiscoveryRequests>,
  profileId: string,
  selectedBefore: string,
) {
  await expect(page.getByTestId("profile-capability-status")).toHaveAttribute(
    "data-status",
    "ready",
    { timeout: 20_000 },
  );
  await expect(page.getByTestId("profile-runtime-info")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Profile start model settings" })).toHaveText(
    selectedBefore,
  );
  const refreshedProbe = requests.find(
    (request) => request.url.endsWith("/probe") && request.body.refresh === true,
  );
  expect(refreshedProbe?.body.profile_id).toBe(profileId);
  expect(refreshedProbe?.body.launch_settings).toMatchObject({
    env_vars: expect.arrayContaining([
      expect.objectContaining({
        key: "MOCK_AGENT_PROFILE_CATALOG",
        value: "mobile-runtime-draft",
      }),
    ]),
  });
}

async function expectNarrowFinePointerRefresh(page: Page, agentName: string, profileId: string) {
  const browser = page.context().browser();
  if (!browser) throw new Error("Browser context is required for the fine-pointer check");
  const fineContext: BrowserContext = await browser.newContext({
    viewport: { width: 767, height: 852 },
    isMobile: false,
    hasTouch: false,
    storageState: await page.context().storageState(),
  });
  try {
    const backendPort = await page.evaluate(() => window.__KANDEV_API_PORT);
    await fineContext.addInitScript(
      ({ backendPort: apiPort }: { backendPort: string }) => {
        window.__KANDEV_API_PORT = apiPort;
      },
      { backendPort },
    );
    const finePage = await fineContext.newPage();
    await installProfileRuntimeObservationFixture(finePage);
    await finePage.goto(
      new URL(
        `/settings/agents/${agentName}/profiles/${profileId}`,
        new URL(page.url()).origin,
      ).toString(),
    );
    expect(await finePage.evaluate(() => matchMedia("(pointer: fine)").matches)).toBe(true);
    const action = finePage.getByTestId("profile-refresh-capabilities");
    await expect(finePage.getByTestId("profile-runtime-info")).toHaveCount(0);
    await expect(action).toBeVisible();
    const bounds = await action.boundingBox();
    expect(bounds?.height).toBeGreaterThanOrEqual(44);
  } finally {
    await fineContext.close();
  }
}

test.describe("Mobile profile capability discovery", () => {
  test("keeps native runtime details out of profile model settings", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is required for profile discovery E2E");
    const { profile } = await createProfileWithCatalog(
      apiClient,
      backend,
      agent.id,
      "Mobile native runtime recovery",
      "mobile-native-runtime",
    );
    await installProfileRuntimeObservationFixture(testPage, { nativeBridge: true });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );

      await expect(testPage.getByTestId("profile-runtime-info")).toHaveCount(0);
      await expect(
        testPage.getByRole("button", { name: "Profile start model settings" }),
      ).toBeVisible();
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("keeps unknown runtime details out of profile model settings", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is required for profile discovery E2E");
    const { profile } = await createProfileWithCatalog(
      apiClient,
      backend,
      agent.id,
      "Mobile prefixed runtime recovery",
      "mobile-prefixed-runtime",
    );
    await installProfileRuntimeObservationFixture(testPage, { unknownManagedFallback: true });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );

      await expect(testPage.getByTestId("profile-runtime-info")).toHaveCount(0);
      await expect(
        testPage.getByRole("button", { name: "Profile start model settings" }),
      ).toBeVisible();
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("refreshes the open draft after a second-tab update without showing runtime details", async ({
    testPage,
    apiClient,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent");
    if (!agent) throw new Error("mock-agent is required for profile discovery E2E");
    const { profile } = await createProfileWithCatalog(
      apiClient,
      backend,
      agent.id,
      "Mobile runtime observation",
      "mobile-runtime-initial",
    );
    const profileRuntime = await installProfileRuntimeObservationFixture(testPage);
    const requests = observeProfileDiscoveryRequests(testPage);
    let updatePage: Page | undefined;

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
      const selector = testPage.getByRole("button", { name: "Profile start model settings" });
      await expect(selector).toBeVisible({ timeout: 15_000 });
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "ready",
        { timeout: 20_000 },
      );

      await expectMobileModelSettings(testPage, prCapture);
      const selectedBefore = await editRuntimeDraft(testPage);

      const updateTab = await approveRuntimeUpdateInOtherTab(testPage, profileRuntime);
      updatePage = updateTab.page;
      await expectActivatedProfileDraft(testPage, requests, profile.id, selectedBefore);
      await selector.tap();
      await expect(
        testPage.getByRole("option", { name: "Profile env after update" }),
      ).toBeVisible();
      await testPage.keyboard.press("Escape");
      await expectNarrowFinePointerRefresh(testPage, agent.name, profile.id);
    } finally {
      if (updatePage) await updatePage.close();
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

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

      const refresh = testPage.getByTestId("profile-refresh-capabilities");
      await expect(refresh).toHaveCount(1);
      const refreshBounds = await refresh.boundingBox();
      const modelBounds = await selector.boundingBox();
      const modeBounds = await testPage
        .getByTestId("profile-mode-field")
        .getByRole("combobox")
        .boundingBox();
      expect(refreshBounds).not.toBeNull();
      expect(modelBounds).not.toBeNull();
      expect(modeBounds).not.toBeNull();
      expect(refreshBounds!.width).toBeGreaterThanOrEqual(44);
      expect(refreshBounds!.height).toBeGreaterThanOrEqual(44);
      expect(refreshBounds!.y + refreshBounds!.height).toBeCloseTo(
        modelBounds!.y + modelBounds!.height,
        0,
      );
      expect(modeBounds!.y).toBeGreaterThanOrEqual(modelBounds!.y + modelBounds!.height);
      await testPage.getByTestId("env-var-row-0").locator("input").nth(1).fill("mobile-draft");
      await expect(testPage.getByTestId("profile-capability-status")).toHaveAttribute(
        "data-status",
        "stale",
      );
      await expect(testPage.getByTestId("profile-mode-field")).toHaveCount(0);
      await expect(refresh).toHaveCount(1);
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
