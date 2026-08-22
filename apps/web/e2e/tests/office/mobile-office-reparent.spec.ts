import { expect, test } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Office manager reassignment on mobile", () => {
  test("saves a manager change and renders the new parent-child link", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const worker = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Mobile Org Chart Reparent Target",
      role: "worker",
    });
    const workerId = worker.id as string;

    await testPage.goto(`/office/agents/${workerId}/configuration`);
    const reportsTo = testPage.getByRole("combobox", { name: "Reports to" });
    await expect(reportsTo).toBeVisible();
    const reportsToBox = await reportsTo.boundingBox();
    expect(reportsToBox).not.toBeNull();
    expect(reportsToBox!.height).toBeGreaterThanOrEqual(44);

    await reportsTo.click();
    const listbox = testPage.getByRole("listbox");
    await expect(listbox).toBeVisible();
    await listbox.getByRole("option", { name: "CEO" }).click();

    const saved = waitForHttp(testPage, "PATCH", new RegExp(`/api/v1/office/agents/${workerId}$`));
    await testPage.getByRole("button", { name: "Save Configuration" }).click();
    await saved;

    await testPage.getByTestId("app-nav-trigger").click();
    await testPage
      .getByTestId("app-nav-sheet")
      .getByRole("link", { name: /Org chart/i })
      .click();
    await expect(testPage).toHaveURL(/\/office\/workspace\/org$/);
    await expect(officeTopbarTitle(testPage)).toHaveText(/Org/i, {
      timeout: 10_000,
    });
    await expect(
      testPage.getByRole("link", { name: "Mobile Org Chart Reparent Target" }),
    ).toHaveAttribute("data-reports-to", officeSeed.agentId);
  });
});
