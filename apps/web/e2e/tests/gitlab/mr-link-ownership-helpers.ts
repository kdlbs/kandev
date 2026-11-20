import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { Locator, Page, TestInfo } from "@playwright/test";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
import { GITLAB_HOST, GITLAB_PROJECT, gitLabMR } from "../../helpers/gitlab";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { SessionPage } from "../../pages/session-page";

function snapshot(repo: string) {
  return {
    head: execFileSync("git", ["-C", repo, "rev-parse", "HEAD"]),
    status: execFileSync("git", [
      "-C",
      repo,
      "status",
      "--porcelain=v1",
      "-z",
      "--untracked-files=all",
    ]),
  };
}
async function activate(control: Locator, mobile: boolean) {
  await expect(control).toHaveCount(1);
  if (mobile) await control.tap();
  else await control.click();
}
async function openLink(page: Page, taskId: string, mobile: boolean) {
  let surface: Locator;
  if (mobile) {
    surface = page.getByRole("dialog", { name: "Tasks", exact: true });
    if (!(await surface.isVisible())) await page.getByTestId("mobile-task-picker-trigger").tap();
  } else surface = page.getByTestId("task-sidebar");
  const row = surface.locator(`[data-task-row-id="${taskId}"]`);
  await expect(row).toHaveCount(1);
  if (!mobile) await row.hover();
  await activate(row.getByRole("button", { name: "Task actions", exact: true }), mobile);
  const link = page.getByRole("menuitem", { name: "Link", exact: true });
  if (mobile) await link.tap();
  else await link.hover();
  await activate(page.getByRole("menuitem", { name: "GitLab Merge Request", exact: true }), mobile);
  const dialog = page.getByRole("dialog", { name: "Link GitLab merge request", exact: true });
  await expect(dialog).toBeVisible();
  await waitForFiniteAnimations(dialog);
  return dialog;
}

// @covers AC-TASKS-TASK-ACTIONS-MENU-004.8
export async function preserveMRLinkDraft(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  {
    backend,
    info,
    mobile,
    sameTask,
  }: { backend: BackendContext; info: TestInfo; mobile: boolean; sameTask: boolean },
) {
  const seedBefore = snapshot(seed.repositoryPath);
  const root = fs.mkdtempSync(path.join(backend.tmpDir, "mr-link-owner-"));
  const source = path.join(root, "source");
  const origin = path.join(root, "origin.git");
  const env = makeGitEnv(backend.tmpDir);
  const git = (args: string[]) => execFileSync("git", args, { env, stdio: "pipe" });
  let workspace: { id: string; name: string } | undefined;
  const taskIds: string[] = [];
  let sourceBefore: ReturnType<typeof snapshot> | undefined;
  let release = () => {};
  const errors: unknown[] = [];
  let cleanupStart = 0;
  const attempt = async (run: () => unknown) => {
    try {
      await run();
    } catch (error) {
      errors.push(error);
    }
  };
  try {
    git(["init", "--bare", "-b", "main", origin]);
    git(["init", "-b", "main", source]);
    fs.writeFileSync(path.join(source, "README.md"), "Offline MR link ownership fixture\n");
    git(["-C", source, "add", "README.md"]);
    git(["-C", source, "commit", "-m", "Seed MR link ownership"]);
    git(["-C", source, "remote", "add", "origin", `file://${origin}`]);
    git(["-C", source, "push", "origin", "main"]);
    sourceBefore = snapshot(source);
    expect(sourceBefore.status.length).toBe(0);
    workspace = await api.createWorkspace(`MR link ownership ${info.testId}`);
    const workflow = await api.createWorkflow(workspace.id, "MR link ownership", "simple");
    const { steps } = await api.listWorkflowSteps(workflow.id);
    const start = steps.find((step) => step.is_start_step) ?? steps[0];
    const repository = await api.createRepository(workspace.id, source);
    await api.updateRepository(repository.id, {
      provider: "gitlab",
      provider_host: GITLAB_HOST,
      provider_owner: "platform",
      provider_name: "kandev",
      pull_before_worktree: false,
    });
    await api.configureGitLab(workspace.id, GITLAB_HOST);
    await api.mockGitLabAddMRs(workspace.id, GITLAB_PROJECT, [
      gitLabMR(81, "Original MR"),
      gitLabMR(82, "Later MR"),
    ]);
    const options = {
      workflow_id: workflow.id,
      workflow_step_id: start.id,
      repository_ids: [repository.id],
      agent_profile_id: seed.agentProfileId,
      executor_profile_id: seed.worktreeExecutorProfileId,
    };
    const a = await api.createTask(workspace.id, "Original MR link target", options);
    taskIds.push(a.id);
    const b = await api.createTask(workspace.id, "Later MR link target", options);
    taskIds.push(b.id);
    const url = (iid: number) => `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iid}`;
    const associations = async () => {
      const response = await api.rawRequest(
        "GET",
        `/api/v1/gitlab/workspaces/${workspace!.id}/task-mrs`,
      );
      expect(response.status).toBe(200);
      return (await response.json()) as {
        task_mrs: Record<string, Array<{ task_id: string; repository_id: string; mr_url: string }>>;
      };
    };
    let admitted!: () => void;
    const admission = new Promise<void>((resolve) => {
      admitted = resolve;
    });
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const posts: unknown[] = [];
    let first = true;
    await page.route("**/api/v1/gitlab/task-mrs?*", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const request = route.request();
      posts.push(request.postDataJSON());
      expect(new URL(request.url()).searchParams.get("workspace_id")).toBe(workspace!.id);
      // Commit through the real backend before holding its acknowledgement.
      const response = await route.fetch();
      expect(response.status()).toBe(200);
      if (first) {
        first = false;
        admitted();
        await held;
      }
      await route.fulfill({ response });
    });
    await page.goto(`/t/${a.id}`);
    await new SessionPage(page).waitForLoad();
    await expect(page.getByText("Couldn't start a session", { exact: true })).toHaveCount(0);
    const original = await openLink(page, a.id, mobile);
    await original.getByLabel("Merge request URL", { exact: true }).fill(` ${url(81)} `);
    await activate(
      original.getByRole("button", { name: "Link merge request", exact: true }),
      mobile,
    );
    await admission;
    expect(posts).toEqual([{ task_id: a.id, repository_id: repository.id, mr_url: url(81) }]);
    expect(Object.values((await associations()).task_mrs).flat()).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ task_id: a.id, repository_id: repository.id, mr_url: url(81) }),
      ]),
    );
    await activate(original.getByRole("button", { name: "Cancel", exact: true }), mobile);
    await expect(original).toBeHidden();
    const later = await openLink(page, sameTask ? a.id : b.id, mobile);
    const input = later.getByLabel("Merge request URL", { exact: true });
    const raw = `  ${url(82)}  `;
    await activate(input, mobile);
    await input.fill(raw);
    await input.evaluate((element: HTMLInputElement) => element.setSelectionRange(3, 9));
    await expect(input).toBeFocused();
    const settled = page.waitForResponse(
      (res) => res.url().includes("/api/v1/gitlab/task-mrs?") && res.request().method() === "POST",
    );
    release();
    const response = await settled;
    expect(response.status()).toBe(200);
    await response.finished();
    // Confirm the original association is also available to the live task UI.
    await expect
      .poll(() =>
        page.evaluate(
          ({ workspaceId, taskId }) => {
            const state = (
              window as unknown as {
                __KANDEV_E2E_STORE__?: {
                  getState: () => {
                    taskMRs: { byWorkspaceId: Record<string, Record<string, unknown[]>> };
                  };
                };
              }
            ).__KANDEV_E2E_STORE__?.getState();
            return state?.taskMRs.byWorkspaceId[workspaceId]?.[taskId]?.length;
          },
          { workspaceId: workspace.id, taskId: a.id },
        ),
      )
      .toBe(1);
    await expect(input).toHaveValue(raw);
    await expect(input).toBeFocused();
    expect(
      await input.evaluate((el: HTMLInputElement) => [el.selectionStart, el.selectionEnd]),
    ).toEqual([3, 9]);
    await expect(
      later.getByRole("button", { name: "Link merge request", exact: true }),
    ).toBeEnabled();
    await info.attach("preserved-mr-link-draft", {
      body: await page.screenshot({ path: info.outputPath("preserved-mr-link-draft.png") }),
      contentType: "image/png",
    });
    if (sameTask) {
      await activate(later.getByRole("button", { name: "Cancel", exact: true }), mobile);
      await expect(later).toBeHidden();
      expect(posts).toHaveLength(1);
    } else {
      await activate(
        later.getByRole("button", { name: "Link merge request", exact: true }),
        mobile,
      );
      await expect(later).toBeHidden();
      expect(posts).toHaveLength(2);
      expect(posts[1]).toEqual({ task_id: b.id, repository_id: repository.id, mr_url: url(82) });
    }
    await page.reload();
    await new SessionPage(page).waitForLoad();
    const persisted = Object.values((await associations()).task_mrs).flat();
    expect(persisted).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ task_id: a.id, repository_id: repository.id, mr_url: url(81) }),
      ]),
    );
    expect(persisted.some((mr) => mr.task_id === b.id && mr.mr_url === url(82))).toBe(!sameTask);
  } catch (error) {
    errors.push(error);
  } finally {
    cleanupStart = errors.length;
    release();
    await attempt(() => page.unrouteAll({ behavior: "wait" }));
    await attempt(() => page.goto("/"));
    for (const taskId of taskIds)
      await attempt(() => api.deleteTask(taskId, { discardWorktreeChanges: true }));
    if (sourceBefore)
      await attempt(() => {
        const trees = git(["-C", source, "worktree", "list", "--porcelain"]).toString();
        for (const line of trees.split("\n").filter((line) => line.startsWith("worktree "))) {
          const owned = line.slice("worktree ".length);
          if (owned === source) continue;
          expect(owned.startsWith(backend.tmpDir + path.sep)).toBe(true);
          git(["-C", source, "worktree", "remove", "--force", owned]);
        }
        expect(snapshot(source)).toEqual(sourceBefore);
        expect(
          git(["-C", source, "worktree", "list", "--porcelain"])
            .toString()
            .split("\n")
            .filter((line) => line.startsWith("worktree ")),
        ).toEqual([`worktree ${source}`]);
      });
    if (workspace) await attempt(() => api.deleteWorkspace(workspace!.id, workspace!.name));
    await attempt(() => expect(snapshot(seed.repositoryPath)).toEqual(seedBefore));
    await attempt(() => {
      fs.rmSync(root, { recursive: true, force: true });
      expect(fs.existsSync(root)).toBe(false);
    });
  }
  const receipt = {
    workspace,
    taskIds,
    root,
    source,
    explicitExecutor: seed.worktreeExecutorProfileId,
    seedBefore: {
      head: seedBefore.head.toString().trim(),
      statusHex: seedBefore.status.toString("hex"),
    },
    sourceBefore: sourceBefore && {
      head: sourceBefore.head.toString().trim(),
      statusHex: sourceBefore.status.toString("hex"),
    },
    rootRemoved: !fs.existsSync(root),
    primaryFailures: errors.slice(0, cleanupStart).map(String),
    cleanupFailures: errors.slice(cleanupStart).map(String),
  };
  const receiptPath = info.outputPath("owned-fixture-closure.json");
  fs.writeFileSync(receiptPath, JSON.stringify(receipt, null, 2));
  await info.attach("owned-fixture-closure", {
    path: receiptPath,
    contentType: "application/json",
  });
  if (errors.length)
    throw new AggregateError(errors, "MR link ownership scenario or cleanup failed");
}
