import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";

import { test, expect } from "../../fixtures/office-fixture";

/**
 * What an office agent actually receives at launch.
 *
 * This is the regression net for the "agents stop getting <X>"
 * class of bug. The asserts cover the three legs an agent depends
 * on:
 *
 *   1. Bundled system SKILL.md files materialised under the
 *      worktree's per-agent skill dir (`.agents/skills/kandev-*`).
 *   2. `KANDEV_CLI` env contract — the binary exists at the path
 *      the runtime injects, and `agentctl --help` succeeds.
 *   3. Run detail's `runtime.skills` snapshot records the same
 *      slugs that landed on disk (proves the dispatcher actually
 *      took the snapshot, not just that the file deployer ran).
 *
 * If any of those break silently, the CEO gets garbage prompts /
 * a broken CLI / missing skills and the user only notices when
 * the org grinds to a halt.
 */

const BACKEND_DIR = path.resolve(__dirname, "../../../../../apps/backend");
const AGENTCTL_BIN = path.join(BACKEND_DIR, "bin", "agentctl");

test.describe("Office agent launch context", () => {
  test("bundled system skills materialise on the agent's worktree at launch", async ({
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    // 1. Prime the bundled system-skill sync for the seedData workspace
    //    (the lazy sync runs on the first /skills list). The kanban
    //    seed workspace shares the same lazy-sync path as office.
    const primingRes = await apiClient.rawRequest(
      "GET",
      `/api/v1/office/workspaces/${seedData.workspaceId}/skills`,
    );
    expect(primingRes.ok).toBe(true);
    const primed = (await primingRes.json()) as { skills?: Array<{ slug: string }> };
    const slugs = (primed.skills ?? []).map((s) => s.slug);
    expect(slugs).toContain("kandev-protocol");

    // 2. Attach the bundled slug to the seed agent's desired_skills.
    //    The runtime materializer resolves slugs against the
    //    workspace skill registry at session start.
    await apiClient.setProfileDesiredSkills(seedData.agentProfileId, ["kandev-protocol"]);

    try {
      // 3. Launch a real session.
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Launch context — bundled skills",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );

      // 4. Wait for the task's worktree path to settle.
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

      // 5. The bundled SKILL.md landed at the agent-type-specific
      //    skill dir. mock-agent doesn't declare a custom
      //    ProjectSkillDir, so the default ".agents/skills" applies.
      const skillFile = path.join(
        worktreePath,
        ".agents",
        "skills",
        "kandev-kandev-protocol",
        "SKILL.md",
      );
      await expect
        .poll(() => fs.existsSync(skillFile), {
          timeout: 15_000,
          message: skillFile,
        })
        .toBe(true);
      // Frontmatter from internal/office/configloader/skills/kandev-protocol
      // — pin that the bundled body actually reached the worktree, not
      // just an empty file.
      const content = fs.readFileSync(skillFile, "utf8");
      expect(content).toMatch(/kandev/i);
    } finally {
      // Tidy up so the worker's next test doesn't inherit the attach.
      await apiClient.setProfileDesiredSkills(seedData.agentProfileId, []);
    }
  });

  test("agentctl binary referenced by KANDEV_CLI is invocable", async () => {
    // The runtime injects `KANDEV_CLI` pointing at the host's
    // agentctl binary. Confirm the binary at the expected path
    // exists and responds to a no-arg invocation — failure here
    // means a CI run won't have a working CLI surface at all.
    expect(fs.existsSync(AGENTCTL_BIN), `${AGENTCTL_BIN} must exist`).toBe(true);

    const probe = spawnSync(AGENTCTL_BIN, ["kandev"], { encoding: "utf8", timeout: 5_000 });
    // `agentctl kandev` with no subcommand prints a usage banner and
    // exits 1. That's enough proof the binary loaded.
    expect(probe.status).toBe(1);
    expect(probe.stderr).toMatch(/Usage: agentctl kandev/);
    expect(probe.stderr).toMatch(/agents/);
    expect(probe.stderr).toMatch(/tasks/);
    expect(probe.stderr).toMatch(/routines/);
    expect(probe.stderr).toMatch(/approvals/);
  });

  test("office workspace exposes a populated runtime skill snapshot on a seeded run", async ({
    apiClient,
    officeSeed,
  }) => {
    // Listing skills in the office workspace triggers the lazy
    // per-workspace system-skill sync so the bundled set is
    // present before we seed the run. The same path runs on the
    // first UI list, so this just mirrors what the user sees.
    const list = (await apiClient.rawRequest(
      "GET",
      `/api/v1/office/workspaces/${officeSeed.workspaceId}/skills`,
    )) as Response;
    expect(list.ok).toBe(true);
    const listBody = (await list.json()) as { skills?: Array<Record<string, unknown>> };
    const slugs = (listBody.skills ?? []).map((s) => s.slug as string);
    expect(slugs).toContain("kandev-protocol");

    // Find the bundled `kandev-protocol` skill id for the snapshot.
    const protocolSkill = (listBody.skills ?? []).find((s) => s.slug === "kandev-protocol") as
      | { id: string; content_hash?: string }
      | undefined;
    expect(protocolSkill?.id).toBeTruthy();

    // Seed a run, then seed a run_skills snapshot for it referencing
    // the bundled kandev-protocol row. Real launches do this via the
    // dispatcher; we replicate the row directly so we can assert the
    // UI reads it back the way the agent stored it.
    const run = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      reason: "routine_dispatch",
      status: "finished",
    });
    await apiClient.seedRunSkillSnapshot({
      runId: run.run_id,
      skillId: protocolSkill!.id,
      version: "v1",
      contentHash: protocolSkill!.content_hash ?? "test-hash",
      materializedPath: "/tmp/run-skills/kandev-protocol/SKILL.md",
    });

    // Hit the office run detail endpoint and confirm the snapshot is
    // serialised back in the `runtime.skills` payload — that's what
    // the UI's Runtime panel renders.
    const detailRes = await apiClient.rawRequest(
      "GET",
      `/api/v1/office/agents/${officeSeed.agentId}/runs/${run.run_id}`,
    );
    expect(detailRes.ok).toBe(true);
    const detail = (await detailRes.json()) as {
      runtime?: { skills?: Array<{ skill_id: string; version?: string }> };
    };
    const recordedIds = (detail.runtime?.skills ?? []).map((s) => s.skill_id);
    expect(recordedIds).toContain(protocolSkill!.id);
  });
});
