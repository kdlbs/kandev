import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import { execFileSync } from "node:child_process";
import { expect, type Page, type CDPSession, type TestInfo } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import type { CreateTaskResponse } from "../../../lib/types/http";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { NavigationResponseGate } from "../../helpers/navigation-response-hold";
import { SessionPage } from "../../pages/session-page";
import {
  AVAILABLE,
  HELD,
  seedNavigationTasks,
  saveExpandedPaths,
  selectNavigationTask,
} from "./task-navigation-helpers";

export const PROFILE_FOLDERS = [
  AVAILABLE,
  HELD,
  ...Array.from({ length: 18 }, (_, i) => `profile-${String(i).padStart(2, "0")}`),
  `${HELD}/nested`,
];

export async function seedProfile(api: ApiClient, seed: SeedData, backend: BackendContext) {
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  for (const folder of PROFILE_FOLDERS)
    for (let i = 0; i < 30; i++)
      git.createFile(
        `${folder}/entry-${String(i).padStart(3, "0")}.ts`,
        "export const before = true;\n",
      );
  git.stageAll();
  if (git.exec("git status --short").trim()) git.commit("seed navigation profile tree");
  for (let i = 0; i < 30; i++)
    await api.createTask(seed.workspaceId, `Profile background ${i}`, {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
    });
  return seedNavigationTasks(api, seed, backend, {
    executorProfileId: seed.worktreeExecutorProfileId,
  });
}

export async function prepareProfileTask(
  page: Page,
  api: ApiClient,
  task: CreateTaskResponse,
  backend: BackendContext,
  first: boolean,
) {
  if (first) await page.goto(`/t/${task.id}`);
  else await selectNavigationTask(page, task.title);
  await expect(page).toHaveURL(new RegExp(`/t/${task.id}$`));
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  const environment = await api.getTaskEnvironment(task.id);
  const repoPath =
    environment?.repos?.[0]?.worktree_path ??
    environment?.worktree_path ??
    environment?.workspace_path;
  if (!repoPath?.startsWith(`${backend.tmpDir}${path.sep}`))
    throw new Error("profile fixture did not expose its owned worktree");
  const git = new GitHelper(repoPath, makeGitEnv(backend.tmpDir));
  git.stageAll();
  if (git.exec("git status --short").trim()) git.commit("profile mock baseline");
  for (let i = 0; i < 72; i++)
    git.modifyFile(
      `${PROFILE_FOLDERS[Math.floor(i / 30)]}/entry-${String(i % 30).padStart(3, "0")}.ts`,
      `export const changed = ${i};\n`,
    );
  const marker = `${AVAILABLE}/000-${task.title.replaceAll(" ", "-")}.txt`;
  git.createFile(marker, "navigation marker\n");
  await api.seedAgentMessages(task.session_id!, 200, `Profile conversation ${task.title}`);
  await session.clickTab("Changes");
  await expect(
    page.getByRole("button", { name: path.basename(marker), exact: true }),
  ).toBeVisible();
  await session.clickTab("Files");
  await expect(session.fileTreeNode(AVAILABLE)).toBeVisible();
  await saveExpandedPaths(page, task.session_id!, PROFILE_FOLDERS);
  return {
    task,
    marker,
    changedFiles: git.exec("git status --porcelain").trim().split("\n").length,
    messageCount: (await api.listSessionMessages(task.session_id!)).messages.length,
  };
}

export async function measureSelection(
  page: Page,
  gate: NavigationResponseGate,
  item: Awaited<ReturnType<typeof prepareProfileTask>>,
) {
  const cursor = gate.requests.length;
  const start = performance.now();
  await selectNavigationTask(page, item.task.title);
  await expect(page).toHaveURL(new RegExp(`/t/${item.task.id}$`));
  const selectedMs = performance.now() - start;
  await new SessionPage(page).clickTab("Files");
  await expect(new SessionPage(page).fileTreeNode(item.marker)).toBeVisible();
  const usableDomMs = performance.now() - start;
  await page.evaluate(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  );
  const paintOpportunityMs = performance.now() - start;
  await expect
    .poll(() => {
      const delivered = new Set(
        gate.requests
          .slice(cursor)
          .filter(
            (r) =>
              r.action === "workspace.tree.get" &&
              r.payload.session_id === item.task.session_id &&
              r.deliveredAt,
          )
          .map((r) => r.payload.path),
      );
      return PROFILE_FOLDERS.every((folder) => delivered.has(folder));
    })
    .toBe(true);
  const restoredMs = performance.now() - start;
  const counts: Record<string, number> = {};
  const keys: Record<string, number> = {};
  for (const r of gate.requests.slice(cursor)) {
    counts[r.action] = (counts[r.action] ?? 0) + 1;
    const key = JSON.stringify([r.action, r.payload]);
    keys[key] = (keys[key] ?? 0) + 1;
  }
  return {
    task: item.task.title,
    selectedMs,
    usableDomMs,
    paintOpportunityMs,
    restoredMs,
    counts,
    keys,
  };
}

export async function sampleMemory(
  page: Page,
  cdp: CDPSession,
  backend: BackendContext,
  info: TestInfo,
  name: string,
) {
  await cdp.send("HeapProfiler.collectGarbage");
  const browser = await cdp.send("Runtime.getHeapUsage");
  const heap = await page.request.get(`${backend.baseUrl}/debug/pprof/heap?gc=1`);
  if (heap.ok() && !heap.headers()["content-type"]?.includes("text/html"))
    await saveProfileArtifact(info, `${name}-go-heap`, await heap.body(), "pb.gz");
  const vars = await page.request.get(`${backend.baseUrl}/debug/vars`);
  const go =
    vars.ok() && vars.headers()["content-type"]?.includes("application/json")
      ? (await vars.json()).memstats
      : { available: false, status: vars.status(), contentType: vars.headers()["content-type"] };
  const pid = backend.pid();
  const status =
    pid && fs.existsSync(`/proc/${pid}/status`)
      ? fs.readFileSync(`/proc/${pid}/status`, "utf8")
      : "";
  return {
    browser,
    go,
    heapStatus: heap.status(),
    heapContentType: heap.headers()["content-type"],
    rssKiB: Number(status.match(/^VmRSS:\s+(\d+)/m)?.[1] ?? 0),
  };
}

export function profileMachine() {
  return {
    revision: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
    cpu: os.cpus()[0]?.model,
    cores: os.cpus().length,
    load: os.loadavg(),
    totalMemory: os.totalmem(),
    freeMemory: os.freemem(),
    platform: `${os.platform()} ${os.release()}`,
  };
}

export async function saveProfileArtifact(
  info: TestInfo,
  name: string,
  body: string | Buffer,
  extension = "json",
) {
  const directory = path.resolve(
    __dirname,
    "../../../../../.kandev/diagnostics/navigation-profile",
  );
  fs.mkdirSync(directory, { recursive: true });
  const filename = `${info.title.replace(/[^a-zA-Z0-9]/g, "-")}-${name}.${extension}`;
  const destination = path.join(directory, filename);
  fs.writeFileSync(destination, body);
  await info.attach(name, { path: destination });
}
