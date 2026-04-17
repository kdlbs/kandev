import { test, expect } from "../../fixtures/test-base";

/**
 * Verifies the Utility Agents settings page loads and renders correctly.
 *
 * This test catches regressions like #611 where the page crashed because
 * the backend stopped returning models in the inference-agents response.
 */
test.describe("Utility Agents settings page", () => {
  test("page loads without crashing and displays sections", async ({ testPage }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/utility-agents");

    // The page should render the main heading (exact match to avoid "Custom utility agents")
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // The default model section should be visible (with agent/model selectors)
    await expect(testPage.getByText(/Default utility agent model/i)).toBeVisible();

    // The actions section should render (built-in utility agents)
    await expect(testPage.getByRole("heading", { name: "Actions" })).toBeVisible();

    // Custom utility agents section should be visible
    await expect(testPage.getByRole("heading", { name: "Custom utility agents" })).toBeVisible();

    // Verify no client-side errors by checking the page didn't crash
    // The page should have the Add button for custom agents
    await expect(testPage.getByRole("button", { name: /Add/i })).toBeVisible();
  });

  test("page survives reload", async ({ testPage }) => {
    test.setTimeout(60_000);

    await testPage.goto("/settings/utility-agents");

    // Wait for initial load (exact match to avoid "Custom utility agents")
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    // Reload the page
    await testPage.reload();

    // Page should still render correctly after reload
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText(/Default utility agent model/i)).toBeVisible();
  });
});
