import { test, expect } from "../../fixtures/office-fixture";

/**
 * Bundled kandev system skills + role-default backfill.
 *
 * The backend ships every SKILL.md under
 * `apps/backend/internal/office/configloader/skills/<slug>/` via
 * `//go:embed`. On startup (and lazily on first list per workspace)
 * the office service upserts each one into `office_skills` with
 * `is_system = true`. New agents inherit `default_for_roles` matches
 * via the `seedDefaultSkills` path; existing agents are backfilled
 * by `BackfillDefaultSkillsForWorkspace`. These specs pin the
 * end-to-end contract so a future SKILL.md rename / frontmatter
 * edit / sync-wiring regression surfaces immediately.
 */
test.describe("Office system skills", () => {
  test("bundled system skills land in the workspace's skill list with is_system=true", async ({
    officeApi,
    officeSeed,
  }) => {
    const res = (await officeApi.listSkills(officeSeed.workspaceId)) as {
      skills?: Array<Record<string, unknown>>;
    };
    const skills = res.skills ?? [];

    // Spot-check several v1 slugs across the role-default tiers so a
    // bulk rename (e.g. kandev-* prefix drop) trips the assertion.
    const expectSlugs = ["kandev-protocol", "memory", "kandev-team-admin", "kandev-task-ops"];
    for (const slug of expectSlugs) {
      const row = skills.find((s) => s.slug === slug);
      expect(row, `expected bundled skill ${slug} in workspace skill list`).toBeTruthy();
      expect(row?.is_system, `${slug}.is_system`).toBe(true);
      expect(typeof row?.system_version).toBe("string");
    }

    // Sanity: nothing outside the kandev-* / memory naming convention
    // should have is_system flipped (no user skill leaked into the set).
    for (const row of skills) {
      if (!row.is_system) continue;
      const slug = String(row.slug ?? "");
      expect(
        slug === "memory" || slug.startsWith("kandev-"),
        `unexpected is_system skill slug: ${slug}`,
      ).toBe(true);
    }
  });

  test("CEO agent inherits role-default system skills on onboarding", async ({
    officeApi,
    officeSeed,
  }) => {
    // Onboarding returns before the asynchronous role-default backfill can
    // finish. Prime the lazy system-skill sync, then read the agent until both
    // persisted skill lists are populated.
    await officeApi.listSkills(officeSeed.workspaceId);
    const expectedDefaultSlugs = [
      "kandev-protocol",
      "memory",
      "kandev-team-admin",
      "kandev-task-ops",
    ];
    let agent: {
      desired_skills?: string;
      skill_ids?: string;
      role?: string;
    } = {};
    await expect
      .poll(
        async () => {
          agent = (await officeApi.getAgent(officeSeed.agentId)) as typeof agent;
          try {
            const desiredSlugs = JSON.parse(agent.desired_skills ?? "[]");
            const desiredIds = JSON.parse(agent.skill_ids ?? "[]");
            return (
              Array.isArray(desiredIds) &&
              desiredIds.length > 0 &&
              Array.isArray(desiredSlugs) &&
              expectedDefaultSlugs.every((slug) => desiredSlugs.includes(slug))
            );
          } catch {
            return false;
          }
        },
        { timeout: 30_000, message: "CEO role-default skills were not backfilled" },
      )
      .toBe(true);
    expect(agent.role).toBe("ceo");

    // After onboarding both `desired_skills` (legacy: slug array,
    // consumed by the runtime materializer) and `skill_ids` (modern:
    // ID array, consumed by the UI toggle) should be populated. The
    // backfill writes both in lock-step.
    const desiredSlugs: string[] = agent.desired_skills ? JSON.parse(agent.desired_skills) : [];
    const desiredIds: string[] = agent.skill_ids ? JSON.parse(agent.skill_ids) : [];
    expect(desiredSlugs.length, "desired_skills").toBeGreaterThan(0);
    expect(desiredIds.length, "skill_ids").toBeGreaterThan(0);

    for (const slug of expectedDefaultSlugs) {
      expect(desiredSlugs, `${slug} must be auto-attached to the CEO`).toContain(slug);
    }
  });

  test("Skills page renders the System group with the bundled set", async ({
    apiClient,
    testPage,
    officeSeed,
  }) => {
    // Prime the workspace's system skills before the page loads.
    // ListSkillsFromConfig triggers the lazy per-workspace sync, so
    // the SSR fetch in page.tsx sees the bundled set on first paint.
    const priming = await apiClient.rawRequest(
      "GET",
      `/api/v1/office/workspaces/${officeSeed.workspaceId}/skills`,
    );
    expect(priming.ok).toBe(true);
    const primed = (await priming.json()) as { skills?: Array<{ slug: string }> };
    expect((primed.skills ?? []).map((s) => s.slug)).toContain("kandev-team-admin");

    await testPage.goto("/office/workspace/skills");

    // The System group is rendered only after the workspace skill list is
    // available. Wait for that state instead of polling a translated count
    // badge, which can be briefly absent while the client store hydrates.
    const systemToggle = testPage.locator('button:has(span:text-is("System"))').first();
    await expect(systemToggle).toBeVisible({ timeout: 15_000 });
    await systemToggle.click();

    await expect(testPage.getByText("kandev-team-admin").first()).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByText("kandev-task-ops").first()).toBeVisible();
  });
});
