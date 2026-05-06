import { test, expect } from "../../fixtures/office-fixture";

test.describe("Office runtime skill snapshots", () => {
  test("run detail keeps the skill hash captured at run start after skill edits", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const suffix = Date.now();
    const skill = await officeApi.createSkill(officeSeed.workspaceId, {
      name: `Runtime Snapshot Skill ${suffix}`,
      slug: `runtime-snapshot-${suffix}`,
      content: "# Runtime Snapshot\n\nOriginal content.",
    });
    const skillId = skill.id as string;
    const seededRun = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "finished",
      reason: "task_assigned",
      capabilities: JSON.stringify({ list_skills: true }),
      inputSnapshot: JSON.stringify({ skill_ids: [skillId] }),
    });
    await apiClient.seedRunSkillSnapshot({
      runId: seededRun.run_id,
      skillId,
      version: "1",
      contentHash: "oldhashabc",
      materializedPath: `/tmp/runtime-skills/${skillId}`,
    });

    await officeApi.updateSkill(skillId, {
      content: "# Runtime Snapshot\n\nUpdated content after run start.",
    });

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${seededRun.run_id}`);
    await expect(testPage.getByTestId("runtime-panel")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("runtime-skills")).toContainText(skillId);
    await expect(testPage.getByTestId("runtime-skills")).toContainText("hash oldhashabc");
    await expect(testPage.getByTestId("runtime-skills")).not.toContainText("Updated content");
  });
});
