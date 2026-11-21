import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { closeModelConfigSheetIfOpen } from "../../helpers/model-config-sheet";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

const START_MODEL_ALIAS = "sonnet";
const FULL_MODEL_ID = "claude-sonnet-5-5";
const CONFLICT_MESSAGE =
  "Choose a full model ID in Start model, or remove ANTHROPIC_MODEL from Environment Variables.";

type Backend = {
  restart(overrides?: Record<string, string>): Promise<void>;
};

export async function verifyClaudeExactModelAliasConflict(
  page: Page,
  apiClient: ApiClient,
  backend: Backend,
  mobile: boolean,
  prCapture?: PrAssetCapture,
): Promise<void> {
  await backend.restart({ KANDEV_MOCK_PROVIDERS: "claude-acp" });
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "claude-acp");
  if (!agent) throw new Error("The E2E fixture must provide a Claude ACP mock agent");
  const profile = await apiClient.createAgentProfile(agent.id, "Claude alias validation", {
    model: START_MODEL_ALIAS,
    fallback_model: "haiku",
    auto_fallback: false,
    require_exact_model: false,
    env_vars: [{ key: "ANTHROPIC_MODEL", value: FULL_MODEL_ID }],
  });

  try {
    await page.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);
    const modelSelector = page.getByRole("button", { name: "Profile start model settings" });
    await expect(modelSelector).toBeVisible({ timeout: 15_000 });
    const fallbackSettings = page.getByTestId("profile-fallback-settings-trigger");
    await fallbackSettings.click();

    const exactSwitch = page.getByRole("switch", { name: "Require exact model" });
    await expect(exactSwitch).toHaveAttribute("data-state", "unchecked");
    if (mobile) await exactSwitch.tap();
    else await exactSwitch.click();
    await expect(exactSwitch).toHaveAttribute("data-state", "checked");

    const envVarValue = page.getByTestId("env-var-row-0").locator("input").nth(1);
    await expect(envVarValue).toBeEnabled();
    await expect(envVarValue).toHaveValue(FULL_MODEL_ID);
    await expect(modelSelector).toBeEnabled();

    const saveButton = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: /^(Save changes|Retry save)$/ });
    if (mobile) await saveButton.tap();
    else await saveButton.click();

    const errorToast = page
      .getByTestId("toast-message")
      .filter({ hasText: "Choose a full model ID in Start model" });
    await expect(errorToast).toBeVisible();
    await expect(errorToast).toContainText(CONFLICT_MESSAGE);
    if (prCapture?.capturing) {
      await waitForFiniteAnimations(page.locator("body"));
      await prCapture.screenshot(
        `claude-exact-model-alias-conflict-${mobile ? "mobile" : "desktop"}`,
        {
          caption: `${mobile ? "Mobile" : "Desktop"} Claude model alias validation message`,
        },
      );
    }
    if (mobile) {
      const viewport = page.viewportSize()!;
      await expect
        .poll(async () => {
          const bounds = await errorToast.boundingBox();
          return bounds !== null && bounds.x + bounds.width <= viewport.width;
        })
        .toBe(true);
      await assertNoDocumentHorizontalOverflow(page, "Claude alias conflict save toast");
      const toastBounds = await errorToast.boundingBox();
      expect(toastBounds).not.toBeNull();
      expect(toastBounds!.x).toBeGreaterThanOrEqual(0);
      expect(toastBounds!.x + toastBounds!.width).toBeLessThanOrEqual(viewport.width);
    }
    await expect(exactSwitch).toHaveAttribute("data-state", "checked");
    await expect(modelSelector).toBeEnabled();
    await expect(envVarValue).toBeEnabled();
    await expect
      .poll(() => apiClient.getAgentProfile(profile.id))
      .toMatchObject({ model: START_MODEL_ALIAS, requireExactModel: false });

    if (mobile) await modelSelector.tap();
    else await modelSelector.click();
    await page.getByPlaceholder("Filter models...").fill(FULL_MODEL_ID);
    const customModel = page.getByTestId("model-config-custom-row");
    await expect(customModel).toBeVisible();
    if (mobile) await customModel.tap();
    else await customModel.click();
    if (mobile) await closeModelConfigSheetIfOpen(page);
    else await page.keyboard.press("Escape");

    const retrySaveButton = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: /^(Save changes|Retry save)$/ });
    await expect(retrySaveButton).toBeEnabled();
    if (mobile) await retrySaveButton.tap();
    else await retrySaveButton.click();
    await expect(page.getByText(/unsaved changes/i)).toBeHidden({ timeout: 15_000 });
    await expect
      .poll(() => apiClient.getAgentProfile(profile.id))
      .toMatchObject({
        model: FULL_MODEL_ID,
        requireExactModel: true,
        fallbackModel: "haiku",
        envVars: [{ key: "ANTHROPIC_MODEL", value: FULL_MODEL_ID }],
      });

    await page.reload();
    await expect(modelSelector).toContainText(FULL_MODEL_ID);
    await expect(fallbackSettings).toBeVisible();
    await fallbackSettings.click();
    await expect(page.getByRole("switch", { name: "Require exact model" })).toHaveAttribute(
      "data-state",
      "checked",
    );
    await expect
      .poll(() => apiClient.getAgentProfile(profile.id))
      .toMatchObject({ model: FULL_MODEL_ID, requireExactModel: true });
    if (mobile) await assertNoDocumentHorizontalOverflow(page, "Claude alias repair after reload");
  } finally {
    await apiClient.deleteAgentProfile(profile.id, true).catch(() => {});
  }
}
