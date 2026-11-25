import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp } from "../../helpers/causal-waits";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

export async function verifyCopilotLongContext(
  {
    backend,
    apiClient,
    testPage,
    prCapture,
  }: {
    backend: BackendContext;
    apiClient: ApiClient;
    testPage: Page;
    prCapture: PrAssetCapture;
  },
  mobile: boolean,
) {
  const restoreBackend = await backend.useEnv({ KANDEV_E2E_MOCK_COPILOT: "true" });
  try {
    const { agents } = await apiClient.listAgents();
    const copilot = agents.find((agent) => agent.name === "copilot-acp");
    expect(copilot).toBeDefined();
    const profile = await apiClient.createAgentProfile(copilot!.id, "Copilot Long Context", {
      model: "",
      cli_flags: [],
    });
    const flag = "--context long_context";
    await testPage.goto(`/settings/agents/copilot-acp/profiles/${profile.id}`);
    const row = testPage.getByTestId("cli-flag-curated-long_context");
    const toggle = row.getByRole("switch");
    const command = testPage.locator("pre code").filter({ hasText: "mock-agent" });
    await expect(toggle).toBeVisible();
    await expect(toggle).not.toBeChecked();
    await expect(command).toBeVisible();
    await expect(command).not.toContainText(flag);

    if (mobile) await toggle.tap();
    else await toggle.click();
    await expect(toggle).toBeChecked();
    await expect(command).toContainText(flag);

    const saved = waitForHttp(
      testPage,
      "PATCH",
      new RegExp(`^/api/v1/agent-profiles/${profile.id}$`),
      {
        predicate: (response) => response.status() === 200,
      },
    );
    const save = testPage.getByRole("button", { name: "Save changes", exact: true });
    if (mobile) await save.tap();
    else await save.click();
    await saved;
    expect((await apiClient.getAgentProfile(profile.id)).cliFlags).toEqual(
      expect.arrayContaining([expect.objectContaining({ flag, enabled: true })]),
    );
    await testPage.reload();
    await expect(toggle).toBeChecked();
    await expect(command).toContainText(flag);

    if (mobile) {
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth === document.documentElement.clientWidth,
        ),
      ).toBe(true);
    }
    if (prCapture.capturing) {
      await row.evaluate((element) => element.scrollIntoView({ block: "center" }));
      await expect(row).toBeInViewport();
      await waitForFiniteAnimations(row);
      await prCapture.screenshot(mobile ? "phone-profile" : "desktop-profile", {
        caption: `Copilot profile with Long context enabled on ${mobile ? "phone" : "desktop"}`,
      });
    }
  } finally {
    await restoreBackend();
  }
}
