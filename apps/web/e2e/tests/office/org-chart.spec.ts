import { test, expect } from "../../fixtures/office-fixture";
import { waitForHttp } from "../../helpers/causal-waits";
import { officeTopbarTitle } from "../../helpers/office-topbar";

async function seededAgentName(
  officeApi: {
    getAgent: (agentId: string) => Promise<Record<string, unknown>>;
  },
  agentId: string,
): Promise<string> {
  const agent = await officeApi.getAgent(agentId);
  const name = agent.name;
  if (typeof name !== "string" || name.length === 0) {
    throw new Error(`office seed agent ${agentId} has no name`);
  }
  return name;
}

test.describe("Org chart", () => {
  test("org chart shows the seeded agent node", async ({ testPage, officeApi, officeSeed }) => {
    const agentName = await seededAgentName(officeApi, officeSeed.agentId);
    await testPage.goto("/office/workspace/org");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Org/i, {
      timeout: 10_000,
    });
    await expect(testPage.getByText(agentName, { exact: true }).first()).toBeVisible({
      timeout: 15_000,
    });
  });

  test("changing an agent's manager on the configuration tab re-parents it on the org chart", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    const agentName = await seededAgentName(officeApi, officeSeed.agentId);
    const worker = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "Org Chart Reparent Target",
      role: "worker",
    });
    const workerId = worker.id as string;

    const agentsLoaded = waitForHttp(
      testPage,
      "GET",
      new RegExp(`/api/v1/office/workspaces/${officeSeed.workspaceId}/agents$`),
    );
    await testPage.goto("/office/workspace/org");
    const agentsResponse = await agentsLoaded;
    expect(agentsResponse.ok()).toBe(true);
    const { agents } = (await agentsResponse.json()) as {
      agents: Array<{ id: string; name: string }>;
    };
    expect(agents).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ id: officeSeed.agentId, name: "CEO" }),
        expect.objectContaining({ id: workerId, name: "Org Chart Reparent Target" }),
      ]),
    );
    await expect(officeTopbarTitle(testPage)).toHaveText(/Org/i, {
      timeout: 10_000,
    });
    const edgesBefore = await testPage.getByTestId("org-edge").count();

    await testPage.goto(`/office/agents/${workerId}/configuration`);
    await testPage.getByRole("combobox", { name: "Reports to" }).click();
    const listbox = testPage.getByRole("listbox");
    await expect(listbox).toBeVisible();
    await listbox.getByRole("option", { name: agentName, exact: true }).click();

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
