import { test, expect } from "../../fixtures/office-fixture";

for (const { role, label, permissions } of [
  {
    role: "security",
    label: "Security",
    permissions: { can_approve: true, can_create_tasks: false },
  },
  { role: "qa", label: "QA", permissions: { can_approve: false, can_create_tasks: true } },
  {
    role: "devops",
    label: "DevOps",
    permissions: { can_approve: false, can_create_tasks: true, can_create_agents: false },
  },
]) {
  test(
    role + " permissions and role badge agree on the list and detail",
    async ({ testPage, officeApi, officeSeed }) => {
      const name = "Role " + label;
      const created = await officeApi.createAgent(officeSeed.workspaceId, { name, role });
      const id = created.id as string;
      expect(id).toBeTruthy();
      try {
        const agent = await officeApi.getAgent(id);
        expect(typeof agent.permissions).toBe("string");
        const actual = JSON.parse(agent.permissions as string);
        expect(actual).toMatchObject(permissions);
        await expect
          .poll(async () => {
            const { agents } = (await officeApi.listAgents(officeSeed.workspaceId)) as {
              agents: Array<{ id: string }>;
            };
            return agents.some((entry) => entry.id === id);
          })
          .toBe(true);
        await testPage.goto("/office/agents");
        const card = testPage.locator("main").locator('a[href="/office/agents/' + id + '"]');
        await expect(card).toHaveCount(1);
        await expect(card.getByText(name, { exact: true })).toBeVisible({ timeout: 20_000 });
        await expect(card.getByText(label, { exact: true })).toBeVisible();
        await card.click();
        await expect(testPage.getByTestId("agent-tab-dashboard")).toBeVisible({
          timeout: 10_000,
        });
        await expect(
          testPage.getByTestId("agent-identity-strip").getByText(label, { exact: true }),
        ).toBeVisible();
      } finally {
        await officeApi.deleteAgent(id);
      }
    },
  );
}
