import { test, expect } from "../../fixtures/office-fixture";

// Office Config Sync (provider-based): a per-workspace GitHub/GitLab source
// that reconciles agents/projects/routines/skills, distinct from the
// filesystem <-> DB diff surface covered by config-sync.spec.ts and the
// raw-git clone/pull/push surface covered by git-section itself. These specs
// exercise the real backend reconciliation engine (internal/office/configsync)
// through a mocked GitLab repository, not a mocked frontend API client.

const GITLAB_PROJECT = "acme/office-config";

test.describe("Office config sync (provider)", () => {
  test("requires a repository target before Save is enabled, and switching provider clears the other target's fields", async ({
    testPage,
  }) => {
    await testPage.goto("/office/workspace/settings");

    const saveBar = testPage.getByTestId("settings-floating-save");
    const saveButton = saveBar.getByRole("button", { name: "Save changes" });
    // Default provider is GitHub; the shared save bar appears after the draft changes.
    await testPage.getByLabel("Repository owner").fill("kdlbs");
    await expect(saveButton).toBeDisabled();
    await testPage.getByLabel("Repository name").fill("office-config");
    await expect(saveButton).toBeEnabled();

    await testPage.getByRole("tab", { name: "GitLab" }).click();
    // Switching provider clears the GitHub-only fields, so Save goes back to
    // disabled until GitLab's own required field (project_path) is filled.
    await expect(saveButton).toBeDisabled();
    const projectPathInput = testPage.getByLabel("Project path");
    await expect(projectPathInput).toBeVisible();
    await expect(projectPathInput).toHaveValue("");
    await projectPathInput.fill(GITLAB_PROJECT);
    await expect(saveButton).toBeEnabled();

    // Never saved, so this test leaves no config-sync source behind for the
    // other tests in this file (they assert on a clean-vs-active state).
  });

  test("reconciles agents, projects, routines and skills from a seeded GitLab repository", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    await apiClient.configureGitLab(officeSeed.workspaceId);
    await apiClient.mockGitLabAddRepoFiles(officeSeed.workspaceId, GITLAB_PROJECT, "main", [
      { path: "agents/e2e-agent.yml", content: "name: E2E Synced Agent\nrole: specialist\n" },
      {
        path: "projects/e2e-project.yml",
        content: "name: E2E Synced Project\ndescription: Synced from GitLab\n",
      },
      {
        path: "routines/e2e-routine.yml",
        content: "name: E2E Synced Routine\ndescription: Synced from GitLab\n",
      },
      {
        path: "skills/e2e-skill/SKILL.md",
        content:
          "---\nname: E2E Synced Skill\ndescription: A skill synced from GitLab\n---\n\n" +
          "# E2E Synced Skill\n\nDo the thing.\n",
      },
      {
        path: "skills/e2e-skill/references/notes.md",
        content: "Reference notes for the synced skill.",
      },
    ]);

    try {
      await officeApi.setConfigSyncConfig(officeSeed.workspaceId, {
        provider: "gitlab",
        project_path: GITLAB_PROJECT,
        branch: "main",
        path: "",
        poll_enabled: false,
      });

      const syncResponse = await officeApi.forceConfigSync(officeSeed.workspaceId);
      expect(syncResponse.error).toBeUndefined();
      expect((syncResponse.result?.created ?? []).slice().sort()).toEqual(
        ["E2E Synced Agent", "E2E Synced Project", "E2E Synced Routine", "e2e-skill"].sort(),
      );

      // Agent-side receipt: assert the actual reconciled rows, not just the
      // sync result's own bookkeeping.
      const agents = (await officeApi.listAgents(officeSeed.workspaceId)) as {
        agents: Array<{ name: string }>;
      };
      expect(agents.agents.some((a) => a.name === "E2E Synced Agent")).toBe(true);

      const projects = (await officeApi.listProjects(officeSeed.workspaceId)) as {
        projects: Array<{ name: string }>;
      };
      expect(projects.projects.some((p) => p.name === "E2E Synced Project")).toBe(true);

      const routines = (await officeApi.listRoutines(officeSeed.workspaceId)) as {
        routines: Array<{ name: string }>;
      };
      expect(routines.routines.some((r) => r.name === "E2E Synced Routine")).toBe(true);

      const skills = (await officeApi.listSkills(officeSeed.workspaceId)) as {
        skills: Array<{ slug: string; name: string; file_inventory: string }>;
      };
      // The list read itself triggers system-skill sync, which canonicalizes
      // any well-formed but non-"kandev-"-prefixed slug (the shared naming
      // rule in internal/common/skillslug) — so the row created from the
      // repo's "e2e-skill" directory is already renamed by the time this
      // response comes back.
      const syncedSkill = skills.skills.find((s) => s.slug === "kandev-e2e-skill");
      expect(syncedSkill?.name).toBe("E2E Synced Skill");
      expect(syncedSkill?.file_inventory ?? "").toContain("notes.md");

      // A second sync over an unchanged repository is idempotent: nothing
      // created, updated or deleted (AC-OFFICE-CONFIG-SYNC-003.11). The
      // backend's Created/Updated/Deleted slices are nil (JSON `null`), not
      // `[]`, when nothing changed — assert length rather than deep-equal a
      // literal empty array, and assert `unchanged` directly since that is
      // the field the AC is actually about.
      const secondSync = await officeApi.forceConfigSync(officeSeed.workspaceId);
      expect(secondSync.error).toBeUndefined();
      expect(secondSync.result?.created ?? []).toHaveLength(0);
      expect(secondSync.result?.updated ?? []).toHaveLength(0);
      expect(secondSync.result?.deleted ?? []).toHaveLength(0);
      expect(secondSync.result?.unchanged).toBe(true);

      await testPage.goto("/office/workspace/settings");
      const status = testPage.getByTestId("office-config-sync-status");
      await expect(status).toBeVisible();
      await expect(status).toHaveAttribute("data-state", "ok");
      await expect(status.getByText(GITLAB_PROJECT)).toBeVisible();
    } finally {
      // Restore a clean, config-sync-free workspace for the other tests in
      // this worker-shared file (and any other office spec sharing the
      // worker), regardless of assertion outcome above.
      await officeApi.deleteConfigSyncConfig(officeSeed.workspaceId);
    }
  });

  test("disables raw-git clone/pull while a config sync source is active, and restores them on removal", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.setConfigSyncConfig(officeSeed.workspaceId, {
      provider: "gitlab",
      project_path: GITLAB_PROJECT,
      branch: "main",
      poll_enabled: false,
    });

    await testPage.goto("/office/workspace/settings");

    // The Clone button is also disabled by an empty repository URL, which is
    // not what this test is about — the guard's own dedicated reason text is
    // the precise signal that config sync (rather than the empty field) is
    // what is disabling it.
    const cloneButton = testPage.getByTestId("office-git-clone");
    await expect(cloneButton).toBeDisabled();
    await expect(testPage.getByTestId("office-git-clone-disabled-reason")).toBeVisible();

    // Remove the config sync source through the real UI flow (not the API),
    // so this also exercises the inline confirm-then-delete path. A
    // successful removal triggers the app's own full-page reload (its
    // `router.refresh()` is a literal `window.location.reload()`, the same
    // as the workflow-sync removal flow), so the load wait must be armed
    // before the click that triggers it, not after.
    await testPage.getByText("Edit configuration").click();
    await testPage.getByTestId("office-config-sync-remove").click();
    await Promise.all([
      testPage.waitForLoadState("load"),
      testPage.getByTestId("office-config-sync-remove-confirm").click(),
    ]);

    // Post-reload: the guard's reason text is gone, and — proving the Clone
    // button itself is genuinely unblocked rather than merely repainted —
    // filling in a repository URL is now enough to enable it.
    await expect(testPage.getByTestId("office-git-clone-disabled-reason")).toHaveCount(0);
    // GitSection's repo-URL label isn't programmatically associated with its
    // input (no htmlFor/id), so target it by its placeholder instead.
    await testPage
      .getByPlaceholder("https://github.com/org/config.git")
      .fill("https://gitlab.com/acme/office-config.git");
    await expect(testPage.getByTestId("office-git-clone")).toBeEnabled();
  });
});
