import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import {
  expect,
  type Locator,
  type Page,
  type TestInfo,
  type WebSocketRoute,
} from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";
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
  if (mobile) await control.tap();
  else await control.click();
}

async function holdCreation(page: Page) {
  let held: { frame: string; id: string; server: WebSocketRoute; payload: unknown } | undefined;
  let response: { success?: boolean; provider?: string; pr_url?: string } | undefined;
  let released = false;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      const forward: string[] = [];
      for (const part of message.toString().split("\n").filter(Boolean)) {
        const frame = JSON.parse(part);
        if (frame.type === "request" && frame.action === "worktree.create_pr" && !held) {
          held = { frame: part, id: frame.id, server, payload: frame.payload };
        } else forward.push(part);
      }
      if (forward.length) server.send(forward.join("\n"));
    });
    server.onMessage((message) => {
      for (const part of message.toString().split("\n").filter(Boolean)) {
        const frame = JSON.parse(part);
        if (held && frame.type === "response" && frame.id === held.id) response = frame.payload;
      }
      socket.send(message);
    });
  });
  return {
    async admitted() {
      await expect
        .poll(() => held?.payload, { message: "native worktree.create_pr admission" })
        .toBeTruthy();
      return held!.payload;
    },
    release() {
      if (held && !released) {
        released = true;
        held.server.send(held.frame);
      }
    },
    async settled() {
      await expect
        .poll(() => response, { timeout: 120_000, message: "real mock-provider create response" })
        .toBeTruthy();
      return response!;
    },
    hasRequest: () => Boolean(held),
  };
}

// @covers AC-INTEGRATIONS-NATIVE-CREATE-001.1, .2, .5
export async function preserveCreationDraft(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  { backend, info, mobile }: { backend: BackendContext; info: TestInfo; mobile: boolean },
) {
  const seedBefore = snapshot(seed.repositoryPath);
  const root = fs.mkdtempSync(path.join(backend.tmpDir, "creation-owner-"));
  const source = path.join(root, "source");
  const origin = path.join(root, "origin.git");
  const env = makeGitEnv(backend.tmpDir);
  const git = (args: string[]) => execFileSync("git", args, { env, stdio: "pipe" });
  let workspace: { id: string; name: string } | undefined;
  let repositoryId: string | undefined;
  let taskId: string | undefined;
  let sourceBefore: ReturnType<typeof snapshot> | undefined;
  const gate = await holdCreation(page);
  const errors: unknown[] = [];
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
    fs.writeFileSync(path.join(source, "README.md"), "Offline creation ownership fixture\n");
    git(["-C", source, "add", "README.md"]);
    git(["-C", source, "commit", "-m", "Seed creation ownership"]);
    git(["-C", source, "remote", "add", "origin", `file://${origin}`]);
    git(["-C", source, "push", "origin", "main"]);
    sourceBefore = snapshot(source);
    expect(sourceBefore.status.length).toBe(0);
    workspace = await api.createWorkspace(`Creation ownership ${info.testId}`);
    const workflow = await api.createWorkflow(workspace.id, "Creation ownership", "simple");
    const { steps } = await api.listWorkflowSteps(workflow.id);
    const start = steps.find((step) => step.is_start_step) ?? steps[0];
    const repository = await api.createRepository(workspace.id, source);
    repositoryId = repository.id;
    await api.configureGitLab(workspace.id, backend.baseUrl);
    await api.configureGitLabRepositoryRemote(
      repository.id,
      `${backend.baseUrl}/platform/kandev.git`,
    );
    await api.updateRepository(repository.id, {
      provider: "gitlab",
      provider_host: backend.baseUrl,
      provider_owner: "platform",
      provider_name: "kandev",
      pull_before_worktree: false,
    });
    const task = await api.createTaskWithAgent(
      workspace.id,
      "Creation ownership",
      seed.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: workflow.id,
        workflow_step_id: start.id,
        repository_ids: [repository.id],
        executor_profile_id: seed.worktreeExecutorProfileId,
      },
    );
    taskId = task.id;
    expect(task.session_id).toBeTruthy();
    await page.goto(`/t/${task.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });
    if (mobile)
      await page
        .getByRole("navigation")
        .getByRole("button", { name: /Changes$/ })
        .tap();
    else await session.clickTab("Changes");
    const changes = page.getByTestId(mobile ? "mobile-changes-panel" : "changes-panel");
    const entry = changes.getByTestId("commits-repo-create-pr");
    await expect(entry).toBeEnabled();
    await activate(entry, mobile);
    const dialog = page.getByRole("dialog", { name: "Create merge request" });
    const title = dialog.getByRole("textbox", { name: "Merge Request title", exact: true });
    const body = dialog.getByRole("textbox", { name: "Description", exact: true });
    await title.fill("Submitted title");
    await body.fill("Submitted body");
    await activate(dialog.getByRole("button", { name: "Create MR", exact: true }), mobile);
    expect(await gate.admitted()).toEqual({
      session_id: task.session_id,
      title: "Submitted title",
      body: "Submitted body",
      base_branch: "",
      draft: true,
      repo: "",
    });
    await expect(dialog).toBeHidden();
    await activate(entry, mobile);
    await title.fill("  New raw title  ");
    await body.fill("  New raw body\n");
    await activate(dialog.getByRole("checkbox", { name: "Create as draft" }), mobile);
    await activate(title, mobile);
    await title.evaluate((element: HTMLInputElement) => element.setSelectionRange(3, 7));
    await expect(title).toBeFocused();
    gate.release();
    expect(await gate.settled()).toMatchObject({ success: true, provider: "gitlab" });
    await expect(
      page.getByTestId("toast-message").filter({ hasText: "Draft MR created" }),
    ).toBeVisible();
    await expect(title).toHaveValue("  New raw title  ");
    await expect(body).toHaveValue("  New raw body\n");
    await expect(dialog.getByRole("checkbox", { name: "Create as draft" })).not.toBeChecked();
    await expect(title).toBeFocused();
    expect(
      await title.evaluate((element: HTMLInputElement) => [
        element.selectionStart,
        element.selectionEnd,
      ]),
    ).toEqual([3, 7]);
    await expect(dialog.getByRole("button", { name: "Create MR", exact: true })).toBeEnabled();
    await info.attach("preserved-create-draft", {
      body: await page.screenshot(),
      contentType: "image/png",
    });
    await activate(dialog.getByRole("button", { name: "Cancel", exact: true }), mobile);
    await expect(dialog).toBeHidden();
    expect((await api.getGitLabPushRecord(repository.id)).args).toBe(
      "push --set-upstream origin HEAD",
    );
  } catch (error) {
    errors.push(error);
  } finally {
    gate.release();
    if (gate.hasRequest()) await attempt(() => gate.settled());
    await attempt(() => page.goto("/"));
    if (taskId) await attempt(() => api.deleteTask(taskId!, { discardWorktreeChanges: true }));
    if (sourceBefore)
      await attempt(() => {
        const worktrees = git(["-C", source, "worktree", "list", "--porcelain"]).toString();
        for (const line of worktrees.split("\n").filter((line) => line.startsWith("worktree "))) {
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
    if (repositoryId) await attempt(() => api.clearGitLabRepositoryRemote(repositoryId!));
    if (workspace) await attempt(() => api.deleteWorkspace(workspace!.id, workspace!.name));
    await attempt(() => expect(snapshot(seed.repositoryPath)).toEqual(seedBefore));
    await attempt(() => {
      fs.rmSync(root, { recursive: true, force: true });
      expect(fs.existsSync(root)).toBe(false);
    });
  }
  if (errors.length)
    throw new AggregateError(errors, "Creation ownership scenario or cleanup failed");
}
