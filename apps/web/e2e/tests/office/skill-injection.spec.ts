import fs from "node:fs";
import path from "node:path";

import { test, expect } from "../../fixtures/test-base";
import { OfficeApiClient } from "../../helpers/office-api-client";

// Verifies the spec-aligned skill injection (docs/specs/office-skills/spec.md):
// when a skill is assigned to an agent profile, launching a session writes
// it to <worktree>/<projectSkillDir>/kandev-<slug>/SKILL.md and appends
// the kandev-* glob to .git/info/exclude.
//
// The mock-agent uses ".agents/skills" as its project-skill dir (it does
// not declare its own RuntimeConfig.ProjectSkillDir, so the default applies).
const PROJECT_SKILL_DIR = ".agents/skills";

test("skill injection writes assigned skill to session worktree on launch", async ({
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(60_000);

  const slug = `e2e-skill-${Date.now()}`;
  const content = `# E2E Skill\n\nMarker: ${slug}-content\n`;

  // 1. Create the skill via office storage (lookup is global, slug-based).
  const officeApi = new OfficeApiClient(backend.baseUrl);
  const skill = (await officeApi.createSkill(seedData.workspaceId, {
    name: "E2E Skill",
    slug,
    content,
  })) as { id: string; slug: string };
  expect(skill.slug).toBe(slug);

  // 2. Assign the skill to the seed agent profile via the test harness.
  //    desired_skills is read from agent_profiles at launch time.
  await apiClient.setProfileDesiredSkills(seedData.agentProfileId, [slug]);

  try {
    // 3. Launch a session via createTaskWithAgent (start_agent: true).
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Skill Injection Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 4. Wait for the task environment with a workspace path. The deployer
    //    writes skills under WorkspacePath; this is the agent's CWD root.
    let worktreePath = "";
    await expect
      .poll(
        async () => {
          const env = await apiClient.getTaskEnvironment(task.id);
          worktreePath = env?.workspace_path ?? env?.worktree_path ?? "";
          return worktreePath;
        },
        { timeout: 30_000, message: "task environment workspace_path never appeared" },
      )
      .not.toBe("");

    // 5. Skill landed at the spec-defined location with the right content.
    const skillFile = path.join(worktreePath, PROJECT_SKILL_DIR, `kandev-${slug}`, "SKILL.md");
    await expect
      .poll(() => fs.existsSync(skillFile), { timeout: 15_000, message: skillFile })
      .toBe(true);
    expect(fs.readFileSync(skillFile, "utf8")).toContain(`Marker: ${slug}-content`);

    // 6. .git/info/exclude (resolved through .git file for linked worktrees)
    //    contains the kandev-* glob so injected skills never get committed.
    const excludePath = resolveGitExcludePath(worktreePath);
    expect(fs.existsSync(excludePath)).toBe(true);
    expect(fs.readFileSync(excludePath, "utf8")).toContain(`${PROJECT_SKILL_DIR}/kandev-*`);
  } finally {
    // Clear desired_skills so this worker's next test isn't affected.
    await apiClient.setProfileDesiredSkills(seedData.agentProfileId, []);
  }
});

function resolveGitExcludePath(worktreePath: string): string {
  const gitPath = path.join(worktreePath, ".git");
  const stat = fs.statSync(gitPath);
  if (stat.isFile()) {
    // linked worktree: ".git" is a "gitdir: <abs>" pointer file
    const text = fs.readFileSync(gitPath, "utf8").trim();
    const match = text.match(/^gitdir:\s*(.+)$/m);
    if (!match) throw new Error(`unparseable .git file: ${text}`);
    return path.join(match[1], "info", "exclude");
  }
  return path.join(gitPath, "info", "exclude");
}
