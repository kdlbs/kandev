import { execFileSync } from "node:child_process";
import {
  mkdtempSync,
  mkdirSync,
  writeFileSync,
  rmSync,
  appendFileSync,
  existsSync,
  readFileSync,
  lstatSync,
  readlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { randomUUID, createHash } from "node:crypto";
import { expect, type TestInfo } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { toAgentProfileOption } from "../../../lib/state/slices/settings/types";
import { makeGitEnv } from "../../helpers/git-helper";

function git(directory: string, args: string[], env?: NodeJS.ProcessEnv) {
  return execFileSync("git", ["-C", directory, ...args], { env, maxBuffer: 16 * 1024 * 1024 });
}
function contentDigest(directory: string) {
  const files = git(directory, ["ls-files", "--cached", "--others", "--exclude-standard", "-z"])
    .toString()
    .split("\0")
    .filter(Boolean)
    .sort();
  const hash = createHash("sha256");
  for (const file of files) {
    const absolute = path.join(directory, file);
    hash.update(file).update("\0");
    if (!existsSync(absolute)) {
      hash.update("deleted\0");
      continue;
    }
    const stat = lstatSync(absolute);
    hash.update(String(stat.mode)).update("\0");
    if (stat.isSymbolicLink()) hash.update(readlinkSync(absolute));
    else if (stat.isFile()) hash.update(readFileSync(absolute));
    hash.update("\0");
  }
  return hash.digest("hex");
}
function snapshot(directory: string) {
  return {
    directory,
    contentSha256: contentDigest(directory),
    head: git(directory, ["rev-parse", "HEAD"]),
    status: git(directory, ["status", "--porcelain=v1", "-z", "--untracked-files=all"]),
  };
}
export type JiraWatchFixture = {
  workspaceId: string;
  name: string;
  workflowId: string;
  stepId: string;
  agentLabel: string;
  profileId: string;
  profileName: string;
  repositoryId: string;
  checkpoint: (phase: string) => void;
};
export async function apiJson<T>(
  api: ApiClient,
  method: string,
  endpoint: string,
  body?: unknown,
): Promise<T> {
  const response = await api.rawRequest(method, endpoint, body);
  expect(response.ok, `${method} ${endpoint}: ${response.status}`).toBe(true);
  const bodyText = await response.text();
  return (bodyText ? JSON.parse(bodyText) : undefined) as T;
}

export async function withJiraWatchFixture(
  api: ApiClient,
  seed: SeedData,
  info: TestInfo,
  run: (fixture: JiraWatchFixture) => Promise<void>,
) {
  const guards = [snapshot(path.resolve(process.cwd(), "../..")), snapshot(seed.repositoryPath)];
  const root = mkdtempSync(path.join(tmpdir(), "jira-watch-ownership-"));
  const source = path.join(root, "source");
  const origin = path.join(root, "origin.git");
  const env = makeGitEnv(root);
  const name = `Watch ownership ${randomUUID()}`;
  const cleanupErrors: unknown[] = [];
  const checkpoints: { phase: string; snapshots: ReturnType<typeof snapshot>[] }[] = [];
  let primary: unknown;
  let workspaceId: string | undefined;
  let profileId: string | undefined;
  const cleanup = async (action: () => Promise<unknown> | unknown) => {
    try {
      await action();
    } catch (error) {
      cleanupErrors.push(error);
    }
  };
  const check = (phase: string) => {
    const snapshots = guards.map((g) => snapshot(g.directory));
    checkpoints.push({ phase, snapshots });
    for (const [index, before] of guards.entries()) {
      const after = snapshots[index];
      expect(after.contentSha256, `${before.directory} complete source bytes preserved`).toBe(
        before.contentSha256,
      );
      expect(after.head.equals(before.head), `${before.directory} HEAD preserved`).toBe(true);
      expect(
        after.status.equals(before.status),
        `${before.directory} full raw porcelain preserved`,
      ).toBe(true);
    }
  };
  try {
    mkdirSync(source);
    git(source, ["init", "-b", "main"], env);
    writeFileSync(path.join(source, "README.md"), "Disposable Jira watch fixture\n");
    git(source, ["add", "README.md"], env);
    git(source, ["commit", "-m", "test: seed owned watch fixture"], env);
    git(root, ["init", "--bare", origin], env);
    git(source, ["remote", "add", "origin", origin], env);
    git(source, ["push", "origin", "main"], env);
    guards.push(snapshot(source));
    const workspace = await api.createWorkspace(name);
    workspaceId = workspace.id;
    await api.setJiraConfig({
      workspaceId,
      siteUrl: "https://jira.ownership.test",
      email: "fixture@example.test",
      secret: "synthetic-jira-watch-fixture",
      defaultProjectKey: "OWN",
    });
    const workflow = await api.createWorkflow(workspaceId, name);
    const step = await api.createWorkflowStep(workflow.id, `${name} step`, 0);
    const { executors } = await api.listExecutors();
    const worktree = executors.find((executor) => executor.type === "worktree");
    expect(worktree, "explicit Worktree executor exists").toBeDefined();
    const profile = await api.createExecutorProfile(worktree!.id, `${name} Worktree`);
    profileId = profile.id;
    const repository = await api.createRepository(workspaceId, source, "main");
    const { agents } = await api.listAgents();
    const agent = agents.find((item) =>
      item.profiles.some((profile) => profile.id === seed.agentProfileId),
    );
    expect(agent).toBeDefined();
    const agentProfile = agent!.profiles.find((profile) => profile.id === seed.agentProfileId)!;
    const agentLabel = toAgentProfileOption(agent!, agentProfile).label;
    checkpoints.push({ phase: "before-flow", snapshots: guards.map((g) => snapshot(g.directory)) });
    await run({
      workspaceId,
      name,
      workflowId: workflow.id,
      stepId: step.id,
      agentLabel,
      profileId,
      profileName: profile.name,
      repositoryId: repository.id,
      checkpoint: check,
    });
  } catch (error) {
    primary = error;
  } finally {
    // Check before and after API cleanup; never repair a checkout to conceal a write.
    await cleanup(() => check("after-flow"));
    if (workspaceId)
      await cleanup(async () => {
        const { watches } = await apiJson<{ watches: { id: string }[] | null }>(
          api,
          "GET",
          `/api/v1/jira/watches/issue?workspace_id=${workspaceId}`,
        );
        for (const watch of watches ?? [])
          await apiJson(
            api,
            "DELETE",
            `/api/v1/jira/watches/issue/${watch.id}?workspace_id=${workspaceId}`,
          );
      });
    if (workspaceId)
      await cleanup(() =>
        apiJson(api, "DELETE", `/api/v1/jira/config?workspace_id=${workspaceId}`),
      );
    if (workspaceId) await cleanup(() => api.deleteWorkspace(workspaceId!, name));
    // Workspace deletion is soft; profiles have an independent owner.
    if (profileId) await cleanup(() => api.deleteExecutorProfile(profileId!));
    await cleanup(async () => {
      const { executors } = await api.listExecutors();
      expect(
        executors
          .flatMap((item) => item.profiles ?? [])
          .some((profile) => profile.id === profileId),
      ).toBe(false);
      const { workspaces } = await api.listWorkspaces();
      expect(workspaces.some((item) => item.id === workspaceId)).toBe(false);
    });
    await cleanup(() => check("after-api-cleanup"));
    await cleanup(() => rmSync(root, { recursive: true, force: true }));
    const receipt = {
      name,
      workspaceId,
      profileId,
      root,
      rootRemoved: !existsSync(root),
      checkpoints: checkpoints.map((c) => ({
        phase: c.phase,
        snapshots: c.snapshots.map((g) => ({
          path: g.directory,
          contentSha256: g.contentSha256,
          head: g.head.toString().trim(),
          fullPorcelainBase64: g.status.toString("base64"),
        })),
      })),
      primary: primary instanceof Error ? primary.message : (primary ?? null),
      cleanupErrors: cleanupErrors.map((error) => String(error)),
      guards: guards.map((guard) => ({
        path: guard.directory,
        contentSha256: guard.contentSha256,
        head: guard.head.toString().trim(),
        fullPorcelainBase64: guard.status.toString("base64"),
      })),
    };
    await cleanup(() =>
      info.attach("watch-ownership-cleanup", {
        body: JSON.stringify(receipt, null, 2),
        contentType: "application/json",
      }),
    );
    if (process.env.KANDEV_JIRA_WATCH_RECEIPTS) {
      await cleanup(() =>
        appendFileSync(process.env.KANDEV_JIRA_WATCH_RECEIPTS!, JSON.stringify(receipt) + "\n"),
      );
    }
  }
  if (primary && cleanupErrors.length === 0) throw primary;
  if (primary || cleanupErrors.length)
    throw new AggregateError(
      [...(primary ? [primary] : []), ...cleanupErrors],
      "Jira watch primary and cleanup failures",
    );
}
