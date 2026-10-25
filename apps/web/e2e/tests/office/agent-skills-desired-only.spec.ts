import { test, expect } from "../../fixtures/office-fixture";

/**
 * Office agent skills — desired-only convergence.
 *
 * Regression: `agent-skills-tab.tsx` seeded checkbox state from
 * `agent.skillIds` alone. An agent whose skill was recorded only in
 * `desired_skills` (slugs) — e.g. seeded via office config import, or from
 * before `skill_ids` existed — showed every such toggle unchecked even
 * though the runtime already unions both columns and the skill is attached
 * and working. Unchecking it was also silently ineffective: `handleSave`
 * only ever sent `skillIds`, so the `desired_skills` slug survived the save
 * and the skill kept materializing at launch anyway. `skill-detail.tsx`'s
 * "Used by" count had the same uuid-vs-slug mismatch and always read 0 for
 * these agents. These specs pin the real backend round-trip for both.
 */
test.describe("Office agent skills - desired-only convergence", () => {
  test("a desired-only skill renders checked, and unchecking it converges both columns", async ({
    officeApi,
    testPage,
    officeSeed,
  }) => {
    const unique = `e2e-desired-only-${Date.now()}`;
    const skill = (await officeApi.createSkill(officeSeed.workspaceId, {
      name: `E2E Desired Only ${unique}`,
      slug: unique,
      content: "# Desired-only\n\nWorkspace-scope skill for e2e coverage.\n",
    })) as { id: string; slug: string };

    // Seed desired_skills directly (bypassing skill_ids) to reproduce an
    // agent whose skill is recorded only as a slug. Preserve the CEO's
    // existing desired_skills rather than clobbering them — this agent is
    // worker-scoped and shared with sibling specs.
    const before = await officeApi.getAgent(officeSeed.agentId);
    const beforeDesired = JSON.parse(
      (before.desired_skills as string | undefined) ?? "[]",
    ) as string[];
    // Save now writes skill_ids too (that's the fix under test), so the
    // restore must snapshot and reset both columns — not desired_skills
    // alone — or this worker-scoped seeded agent leaks a converged
    // skill_ids into every other spec in the worker.
    const beforeIds = JSON.parse((before.skill_ids as string | undefined) ?? "[]") as string[];

    try {
      await officeApi.updateAgent(officeSeed.agentId, {
        desired_skills: JSON.stringify([...beforeDesired, skill.slug]),
      });

      await testPage.goto(`/office/agents/${officeSeed.agentId}/skills`);
      await expect(testPage.getByTestId("agent-tab-skills")).toBeVisible({ timeout: 10_000 });

      const checkbox = testPage.getByTestId(`skill-toggle-checkbox-${skill.slug}`);
      await expect(checkbox).toHaveAttribute("data-state", "checked", { timeout: 10_000 });

      // Uncheck it, save, and wait for the PATCH to land.
      await checkbox.click();
      await expect(checkbox).toHaveAttribute("data-state", "unchecked");
      const saveResponse = testPage.waitForResponse(
        (resp) =>
          resp.url().includes(`/api/v1/office/agents/${officeSeed.agentId}`) &&
          resp.request().method() === "PATCH" &&
          resp.status() === 200,
        { timeout: 10_000 },
      );
      await testPage.getByRole("button", { name: /save skills/i }).click();
      await saveResponse;

      // The server record must no longer carry the slug in EITHER column —
      // proving the fix converged the columns, not just the UI's optimistic
      // local state.
      const after = await officeApi.getAgent(officeSeed.agentId);
      const afterDesired = JSON.parse(
        (after.desired_skills as string | undefined) ?? "[]",
      ) as string[];
      const afterIds = JSON.parse((after.skill_ids as string | undefined) ?? "[]") as string[];
      expect(afterDesired).not.toContain(skill.slug);
      expect(afterIds).not.toContain(skill.id);

      // Reload from a cold store: the checkbox stays unchecked, proving the
      // fix persisted rather than being an artifact of client-only state.
      await testPage.reload();
      await expect(testPage.getByTestId(`skill-toggle-checkbox-${skill.slug}`)).toHaveAttribute(
        "data-state",
        "unchecked",
        { timeout: 10_000 },
      );
    } finally {
      await officeApi.updateAgent(officeSeed.agentId, {
        desired_skills: JSON.stringify(beforeDesired),
        skill_ids: JSON.stringify(beforeIds),
      });
    }
  });

  test("skill-detail 'used by' count includes an agent with only a desired-only slug", async ({
    officeApi,
    testPage,
    officeSeed,
  }) => {
    const unique = `e2e-used-by-${Date.now()}`;
    const skill = (await officeApi.createSkill(officeSeed.workspaceId, {
      name: `E2E Used By ${unique}`,
      slug: unique,
      content: "# Used-by\n\nWorkspace-scope skill for e2e coverage.\n",
    })) as { id: string; slug: string };

    const before = await officeApi.getAgent(officeSeed.agentId);
    const beforeDesired = JSON.parse(
      (before.desired_skills as string | undefined) ?? "[]",
    ) as string[];

    try {
      // Attach the skill via desired_skills only — skill_ids is untouched,
      // reproducing the uuid-vs-slug mismatch skill-detail.tsx had.
      await officeApi.updateAgent(officeSeed.agentId, {
        desired_skills: JSON.stringify([...beforeDesired, skill.slug]),
      });

      await testPage.goto("/office/workspace/skills");
      const row = testPage.locator("button", { hasText: `E2E Used By ${unique}` }).first();
      await expect(row).toBeVisible({ timeout: 10_000 });
      await row.click();

      await expect(testPage.getByText(/^1 agent$/i)).toBeVisible({ timeout: 10_000 });
    } finally {
      await officeApi.updateAgent(officeSeed.agentId, {
        desired_skills: JSON.stringify(beforeDesired),
      });
    }
  });
});
