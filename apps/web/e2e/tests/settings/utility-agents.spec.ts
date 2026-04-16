import { test, expect } from "../../fixtures/test-base";

/**
 * Covers the Utility Agents settings page.
 *
 * The first test is a regression guard for a bug where the backend emitted
 * `models: null` on /api/v1/utility/inference-agents. The frontend's flatMap
 * over `ia.models` blew up and crashed the whole settings page during render.
 * The other tests smoke-check the page loads and walk through the main
 * interactions (open the page, inspect sections, open the create dialog).
 */
test.describe("Utility Agents settings page", () => {
  test("does not crash when backend returns models: null", async ({ testPage }) => {
    // Simulate the exact shape the backend used to emit. Guards against a
    // regression where frontend null-deref would take the whole page down.
    await testPage.route("**/api/v1/utility/inference-agents", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          agents: [
            {
              id: "broken-agent",
              name: "broken-agent",
              display_name: "Broken Agent",
              models: null,
            },
          ],
        }),
      }),
    );

    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    await testPage.goto("/settings/utility-agents");

    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });

  test("renders all sections with seeded built-in utility agents", async ({ testPage }) => {
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    await testPage.goto("/settings/utility-agents");

    // Top-level heading + subtitle.
    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(
      testPage.getByText("One-shot AI helpers for commits, PRs, and prompts."),
    ).toBeVisible();

    // Default-model section.
    await expect(
      testPage.getByRole("heading", { name: "Default utility agent model", exact: true }),
    ).toBeVisible();

    // Built-in actions (seeded on first boot — see builtins.go).
    // Assert a representative subset; the full list lives server-side.
    await expect(testPage.getByText("commit-message", { exact: true })).toBeVisible();
    await expect(testPage.getByText("pr-title", { exact: true })).toBeVisible();
    await expect(testPage.getByText("enhance-prompt", { exact: true })).toBeVisible();

    // Custom agents section + empty state.
    await expect(
      testPage.getByRole("heading", { name: "Custom utility agents", exact: true }),
    ).toBeVisible();
    await expect(testPage.getByText("No custom utility agents.")).toBeVisible();

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });

  test("opens the create-agent dialog from the Add button", async ({ testPage }) => {
    const pageErrors: Error[] = [];
    testPage.on("pageerror", (err) => pageErrors.push(err));

    await testPage.goto("/settings/utility-agents");

    await expect(
      testPage.getByRole("heading", { name: "Utility Agents", exact: true }),
    ).toBeVisible({ timeout: 15_000 });

    await testPage.getByRole("button", { name: "Add", exact: true }).click();

    // The dialog is rendered by UtilityAgentDialog; title differs between
    // create and edit mode. We're in create mode here.
    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible({ timeout: 5_000 });
    await expect(dialog.getByText("Create Utility Agent")).toBeVisible();

    // Close the dialog — nothing should explode.
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).not.toBeVisible();

    expect(pageErrors, `uncaught errors: ${pageErrors.map((e) => e.message).join("; ")}`).toEqual(
      [],
    );
  });
});
