import { test, expect } from "../../fixtures/test-base";
import { gotoFeatureToggles } from "./feature-toggles-restart-helpers";

/**
 * Runs against the real backend (nothing stubbed): the per-session MCP token
 * flag is registered, offered as a toggle, and off in e2e.
 */
test.describe("Feature Toggles: session MCP token", () => {
  test("lists the flag as off by default", async ({ testPage }) => {
    const settings = await gotoFeatureToggles(testPage);

    const card = settings.getByTestId("feature-toggle-features.sessionMcpToken");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await expect(card.getByText("Per-session MCP token and Git broker relay")).toBeVisible();
    await expect(
      card.getByRole("switch", { name: /Per-session MCP token and Git broker relay/ }),
    ).not.toBeChecked();
    await expect(card.getByText("KANDEV_FEATURES_SESSION_MCP_TOKEN")).toBeVisible();
  });
});
