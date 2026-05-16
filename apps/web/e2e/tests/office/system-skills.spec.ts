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
    const expectSlugs = ["kandev-protocol", "memory", "kandev-team", "kandev-hiring"];
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
    const agent = (await officeApi.getAgent(officeSeed.agentId)) as {
      desired_skills?: string;
      skill_ids?: string;
      role?: string;
    };
    expect(agent.role).toBe("ceo");

    // After onboarding both `desired_skills` (legacy: slug array,
    // consumed by the runtime materializer) and `skill_ids` (modern:
    // ID array, consumed by the UI toggle) should be populated. The
    // backfill writes both in lock-step.
    const desiredSlugs: string[] = agent.desired_skills ? JSON.parse(agent.desired_skills) : [];
    const desiredIds: string[] = agent.skill_ids ? JSON.parse(agent.skill_ids) : [];
    expect(desiredSlugs.length, "desired_skills").toBeGreaterThan(0);
    expect(desiredIds.length, "skill_ids").toBeGreaterThan(0);

    for (const slug of ["kandev-protocol", "memory", "kandev-team", "kandev-hiring"]) {
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
    expect((primed.skills ?? []).map((s) => s.slug)).toContain("kandev-hiring");

    await testPage.goto("/office/workspace/skills");

    // The count badge shows N≥3 available (3 pre-existing v1 bundled
    // slugs at minimum). Wait specifically for ≥13 — anything lower
    // means the SSR didn't see the synced set.
    await expect
      .poll(
        async () => {
          const badge = await testPage
            .getByText(/\d+ available/)
            .first()
            .textContent();
          const match = badge?.match(/(\d+)/);
          return match ? Number(match[1]) : 0;
        },
        { timeout: 10_000 },
      )
      .toBeGreaterThanOrEqual(10);

    // Expand the System group. With the heading rendered inside a
    // button containing "System" + a count badge as separate spans,
    // we locate the button via its testable text content (chevron
    // + heading + count) — `hasText` matches against a regex that
    // tolerates whitespace and a trailing count of any width.
    const systemToggle = testPage.locator('button:has(span:text-is("System"))').first();
    await expect(systemToggle).toBeVisible({ timeout: 5_000 });
    await systemToggle.click();

    await expect(testPage.getByText("kandev-hiring").first()).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByText("kandev-team").first()).toBeVisible();
  });
});
