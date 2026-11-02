import { execFileSync } from "node:child_process";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync, appendFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, type TestInfo } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { toAgentProfileOption } from "../../../lib/state/slices/settings/types";
import { makeGitEnv } from "../../helpers/git-helper";

function git(directory: string, args: string[], env?: NodeJS.ProcessEnv) {
  return execFileSync("git", ["-C", directory, ...args], { env });
}
function snapshot(directory: string) {
  return {
    directory,
    head: git(directory, ["rev-parse", "HEAD"]),
    status: git(directory, ["status", "--porcelain=v1", "-z", "--untracked-files=all"]),
  };
}
export type SentryWatchFixture = {
  workspaceId: string;
  name: string;
  workflowId: string;
  stepId: string;
  agentLabel: string;
  profileId: string;
  profileName: string;
  repositoryId: string;
  instanceId: string;
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

export async function withSentryWatchFixture(
  api: ApiClient,
  seed: SeedData,
  info: TestInfo,
  run: (fixture: SentryWatchFixture) => Promise<void>,
) {
  const root = mkdtempSync(path.join(tmpdir(), "sentry-watch-ownership-"));
  const source = path.join(root, "source");
  const origin = path.join(root, "origin.git");
  const env = makeGitEnv(root);
  const guards = [snapshot(path.resolve(process.cwd(), "../..")), snapshot(seed.repositoryPath)];
  const name = `Watch ownership ${randomUUID()}`;
  const cleanupErrors: unknown[] = [];
  const checkpoints: { phase: string; snapshots: ReturnType<typeof snapshot>[] }[] = [];
  let primary: unknown;
  let workspaceId: string | undefined;
  let profileId: string | undefined;
  let instanceId: string | undefined;
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
    for (const before of guards) {
      const after = snapshot(before.directory);
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
    writeFileSync(path.join(source, "README.md"), "Disposable Sentry watch fixture\n");
    git(source, ["add", "README.md"], env);
    git(source, ["commit", "-m", "test: seed owned watch fixture"], env);
    git(root, ["init", "--bare", origin], env);
    git(source, ["remote", "add", "origin", origin], env);
    git(source, ["push", "origin", "main"], env);
    guards.push(snapshot(source));
    const workspace = await api.createWorkspace(name);
    workspaceId = workspace.id;
    const instance = await api.createSentryInstance({
      workspaceId,
      name,
      secret: "sntrys_synthetic_fixture",
      url: "https://sentry.ownership.test",
    });
    instanceId = instance.id;
    await api.mockSentrySetAuthHealth({ instanceId, ok: true });
    await api.mockSentrySetOrganizations(instanceId, [
      { id: "owned-org", slug: "owned-org", name: "Owned org" },
    ]);
    await api.mockSentrySetProjects(instanceId, [
      { id: "owned-project", slug: "owned-project", name: "Owned project", orgSlug: "owned-org" },
    ]);
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
      instanceId,
      checkpoint: check,
    });
  } catch (error) {
    primary = error;
  } finally {
    // Check before and after API cleanup; never repair a checkout to conceal a write.
    await cleanup(() => check("after-flow"));
    if (workspaceId)
      await cleanup(async () => {
        const { watches } = await apiJson<{ watches: { id: string }[] }>(
          api,
          "GET",
          `/api/v1/sentry/watches/issue?workspace_id=${workspaceId}`,
        );
        for (const watch of watches)
          await apiJson(
            api,
            "DELETE",
            `/api/v1/sentry/watches/issue/${watch.id}?workspace_id=${workspaceId}`,
          );
      });
    if (workspaceId && instanceId)
      await cleanup(() =>
        apiJson(
          api,
          "DELETE",
          `/api/v1/sentry/instances/${instanceId}?workspace_id=${workspaceId}`,
        ),
      );
    if (workspaceId) await cleanup(() => api.deleteWorkspace(workspaceId!, name));
    // Workspace deletion is soft; profiles have an independent owner.
    if (profileId) await cleanup(() => api.deleteExecutorProfile(profileId!));
    await cleanup(() => check("after-api-cleanup"));
    await cleanup(() => rmSync(root, { recursive: true, force: true }));
    const receipt = {
      name,
      workspaceId,
      profileId,
      instanceId,
      root,
      rootRemoved: !existsSync(root),
      checkpoints: checkpoints.map((c) => ({
        phase: c.phase,
        snapshots: c.snapshots.map((g) => ({
          path: g.directory,
          head: g.head.toString().trim(),
          fullPorcelainBase64: g.status.toString("base64"),
        })),
      })),
      primary: primary instanceof Error ? primary.message : (primary ?? null),
      cleanupErrors: cleanupErrors.map((error) => String(error)),
      guards: guards.map((guard) => ({
        path: guard.directory,
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
    if (process.env.KANDEV_SENTRY_WATCH_RECEIPTS) {
      await cleanup(() =>
        appendFileSync(process.env.KANDEV_SENTRY_WATCH_RECEIPTS!, JSON.stringify(receipt) + "\n"),
      );
    }
  }
  if (primary && cleanupErrors.length === 0) throw primary;
  if (primary || cleanupErrors.length)
    throw new AggregateError(
      [...(primary ? [primary] : []), ...cleanupErrors],
      "Sentry watch primary and cleanup failures",
    );
}
