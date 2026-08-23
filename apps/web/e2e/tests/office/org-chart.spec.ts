import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Org chart", () => {
  test("org chart shows CEO agent node", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/org");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Org/i, {
      timeout: 10_000,
    });
    // CEO agent from onboarding should appear as a node
    await expect(testPage.getByText("CEO").first()).toBeVisible({ timeout: 15_000 });
  });

  test("changing an agent's manager on the configuration tab re-parents it on the org chart", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const worker = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Org Chart Reparent Target",
      role: "worker",
    });
    const workerId = worker.id as string;

    await testPage.goto("/office/workspace/org");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Org/i, {
      timeout: 10_000,
    });
    const edgesBefore = await testPage.getByTestId("org-edge").count();

    await testPage.goto(`/office/agents/${workerId}/configuration`);
    await testPage.getByRole("combobox", { name: "Reports to" }).click();
    const listbox = testPage.getByRole("listbox");
    await expect(listbox).toBeVisible();
    await listbox.getByRole("option", { name: "CEO" }).click();

    const saved = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/agents/${workerId}$`));
    await testPage.getByRole("button", { name: "Save Configuration" }).click();
    await saved;

    await testPage.getByRole("link", { name: /Agent topology/i }).click();
    await expect(officeTopbarTitle(testPage)).toHaveText(/Org/i, {
      timeout: 10_000,
    });
    const orgChart = testPage.getByTestId("org-chart-edges").locator("..");
    await expect(orgChart.getByRole("link", { name: /Org Chart Reparent Target/ })).toHaveAttribute(
      "data-reports-to",
      officeSeed.agentId,
    );
    await expect(testPage.getByTestId("org-edge")).toHaveCount(edgesBefore + 1, {
      timeout: 15_000,
    });
  });
});
