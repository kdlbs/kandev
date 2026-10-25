import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";
import { waitForHttp } from "../../helpers/causal-waits";

// Covers the frontend surface added by REQ-OFFICE-BUDGET-002.9/.12 (daily/
// yearly period options) and REQ-OFFICE-BUDGET-003.5 (built-in default
// ceiling, readable/writable without inspecting the database).
test.describe("Office budget enforcement UI", () => {
  test.beforeEach(async ({ testPage }) => {
    await testPage.goto("/office/workspace/costs");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Costs/i, { timeout: 10_000 });
    await testPage.getByRole("tab", { name: "Budgets" }).click();
  });

  test("default ceiling card shows the built-in default and can be edited", async ({
    testPage,
  }) => {
    const initialLoad = waitForHttp(testPage, "GET", /\/budgets\/default$/);
    await testPage.reload();
    await testPage.getByRole("tab", { name: "Budgets" }).click();
    await initialLoad;

    // Shipped default is 500,000 subcents == $50.00 (AC-OFFICE-BUDGET-003.9)
    // until a workspace overrides it.
    const card = testPage.getByText("Default Ceiling").locator("..").locator("..");
    await expect(card.getByText("$50.00")).toBeVisible();

    await card.getByRole("button", { name: "Edit default ceiling" }).click();
    const input = card.getByRole("spinbutton");
    await input.fill("75.50");

    const saved = waitForHttp(testPage, "PUT", /\/budgets\/default$/, {
      predicate: (r) => r.status() === 200,
    });
    await card.getByRole("button", { name: "Save" }).click();
    await saved;

    await expect(card.getByText("$75.50")).toBeVisible();

    // Reload to prove the write actually persisted server-side, not just in
    // local component state.
    const reloadFetch = waitForHttp(testPage, "GET", /\/budgets\/default$/);
    await testPage.reload();
    await testPage.getByRole("tab", { name: "Budgets" }).click();
    await reloadFetch;
    await expect(
      testPage.getByText("Default Ceiling").locator("..").locator("..").getByText("$75.50"),
    ).toBeVisible();
  });

  test("create-budget period select offers exactly daily, monthly, yearly, total", async ({
    testPage,
  }) => {
    await testPage.getByRole("button", { name: "Add Policy" }).click();

    // Period is the 4th FormField (Scope Type, Scope ID, Limit, Period, ...);
    // scope it via the visible "Period" label rather than a bare role query,
    // since the form has several comboboxes.
    const periodField = testPage
      .locator("div", { has: testPage.getByText("Period", { exact: true }) })
      .last();
    await periodField.getByRole("combobox").click();

    const listbox = testPage.getByRole("listbox");
    await expect(listbox.getByRole("option", { name: "Daily" })).toBeVisible();
    await expect(listbox.getByRole("option", { name: "Monthly" })).toBeVisible();
    await expect(listbox.getByRole("option", { name: "Yearly" })).toBeVisible();
    await expect(listbox.getByRole("option", { name: "Total" })).toBeVisible();
    // Exactly these four - no fifth/unexpected option leaking through.
    await expect(listbox.getByRole("option")).toHaveCount(4);
  });

  test("selecting a daily period in create-budget wires through to the POST body", async ({
    testPage,
    officeSeed,
  }) => {
    await testPage.getByRole("button", { name: "Add Policy" }).click();

    const scopeIdField = testPage
      .locator("div", { has: testPage.getByText("Scope ID", { exact: true }) })
      .last();
    await scopeIdField.getByPlaceholder("Entity ID").fill(officeSeed.workspaceId);

    const limitField = testPage
      .locator("div", { has: testPage.getByText("Limit ($)", { exact: true }) })
      .last();
    await limitField.getByRole("spinbutton").fill("12.34");

    const periodField = testPage
      .locator("div", { has: testPage.getByText("Period", { exact: true }) })
      .last();
    await periodField.getByRole("combobox").click();
    await testPage.getByRole("option", { name: "Daily" }).click();

    const posted = waitForHttp(testPage, "POST", /\/budgets$/);
    await testPage.getByRole("button", { name: "Create Policy" }).click();
    const response = await posted;

    expect(response.status()).toBe(201);
    const requestBody = JSON.parse(response.request().postData() ?? "{}") as Record<
      string,
      unknown
    >;
    expect(requestBody).toMatchObject({
      scope_type: "workspace",
      scope_id: officeSeed.workspaceId,
      limit_subcents: 123400,
      period: "daily",
      alert_threshold_pct: 80,
      action_on_exceed: "notify_only",
    });
  });
});
