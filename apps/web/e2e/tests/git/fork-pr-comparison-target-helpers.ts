import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { createServer, type IncomingMessage, type Server, type ServerResponse } from "node:http";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../../fixtures/backend";
import { resetSeedRepositoryCheckout, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";

const targetURL = "https://github.com/upstream/widget.git";

type ForkPRComparisonOptions = {
  comparisonTargetAvailable?: boolean;
  localUncommittedFile?: string;
};

export type ComparisonTargetAuthFixture = {
  url: string;
  unauthorizedRequestCount: () => number;
  setAvailable: (available: boolean) => void;
  close: () => Promise<void>;
};

type ActiveComparisonTargetFixture = {
  taskID?: string;
  fixture?: ComparisonTargetAuthFixture;
  releaseBackendEnv?: () => Promise<void>;
};

const activeComparisonTargetFixtures = new Map<string, ActiveComparisonTargetFixture>();

export async function seedForkPRComparisonTask(
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
  options: ForkPRComparisonOptions = {},
) {
  const gitEnv = makeGitEnv(backend.tmpDir);
  const suffix = `${process.pid}-${Date.now()}`;
  const targetRemoteDir = path.join(backend.tmpDir, "repos", `upstream-${suffix}.git`);
  const targetWorktreeDir = path.join(backend.tmpDir, "repos", `upstream-${suffix}`);
  const localGit = new GitHelper(seedData.repositoryPath, gitEnv);

  // The seed repository is shared by tests in a worker. A prior git test can
  // leave it on a feature branch or with uncommitted fixture files, which
  // makes the fork checkout below fail before the scenario starts.
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  // A previous scenario can finish while its comparison fetch is still
  // unwinding after the backend restart. Remove its deterministic remote ref
  // again before this scenario creates its target.
  clearComparisonTargetRefs(seedData, backend.tmpDir);

  execFileSync("git", ["clone", "--bare", seedData.repositoryRemoteURL, targetRemoteDir], {
    env: gitEnv,
  });
  execFileSync("git", ["clone", `file://${targetRemoteDir}`, targetWorktreeDir], { env: gitEnv });
  const upstreamGit = new GitHelper(targetWorktreeDir, gitEnv);
  upstreamGit.createFile("upstream-only.txt", "upstream target commit\n");
  upstreamGit.stageAll();
  upstreamGit.commit("Advance upstream target");
  upstreamGit.exec("git push origin main");

  execFileSync("git", ["fetch", `file://${targetRemoteDir}`, "main"], {
    cwd: seedData.repositoryPath,
    env: gitEnv,
  });
  localGit.exec("git checkout -B feature/fork FETCH_HEAD");
  for (const [name, content] of [
    ["fork-one.txt", "fork change one\n"],
    ["fork-two.txt", "fork change two\n"],
    ["fork-three.txt", "fork change three\n"],
  ]) {
    localGit.createFile(name, content);
  }
  localGit.stageAll();
  const headSHA = localGit.commit("Add three fork contribution files");

  if (options.localUncommittedFile) {
    fs.writeFileSync(
      path.join(seedData.repositoryPath, options.localUncommittedFile),
      "local change remains available\n",
    );
  }

  // agentctl fetches the credential-free provider URL. The test maps that URL
  // to its disposable upstream bare repository without changing origin. The
  // unavailable mode uses a local HTTP server which returns 401 until the
  // test restores it, so the scenario proves authentication failure and
  // recovery rather than only a missing path.
  let comparisonTargetURL: string;
  let comparisonTargetFixture: ComparisonTargetAuthFixture | undefined;
  let comparisonExecutorProfileID: string | undefined;
  if (options.comparisonTargetAvailable === false) {
    comparisonTargetFixture = await startComparisonTargetAuthFixture(targetRemoteDir);
    comparisonTargetURL = comparisonTargetFixture.url;
    const comparisonGitConfigPath = path.join(
      backend.tmpDir,
      `comparison-target-${suffix}.gitconfig`,
    );
    execFileSync(
      "git",
      [
        "config",
        "--file",
        comparisonGitConfigPath,
        `url.${comparisonTargetURL}.insteadOf`,
        targetURL,
      ],
      { env: gitEnv },
    );

    let releaseBackendEnv: (() => Promise<void>) | undefined;
    try {
      // The root comparison tracker is created when agentctl starts. Install
      // the rewrite in the backend's inherited environment before the task is
      // launched so that first materialization and later task Git commands use
      // the same disposable target.
      releaseBackendEnv = await backend.useEnv({
        GIT_CONFIG_GLOBAL: comparisonGitConfigPath,
      });
      const { executors } = await apiClient.listExecutors();
      const localExecutor = executors.find(
        (candidate) => candidate.type === "local" || candidate.type === "local_pc",
      );
      if (!localExecutor) {
        throw new Error("comparison target fixture requires a direct local executor");
      }
      const profile = await apiClient.createExecutorProfile(localExecutor.id, {
        name: `E2E comparison target ${suffix}`,
        env_vars: [
          { key: "GIT_CONFIG_COUNT", value: "1" },
          ...gitConfigEnvVarsForURLRewrite(comparisonTargetURL),
        ],
      });
      comparisonExecutorProfileID = profile.id;
      activeComparisonTargetFixtures.set(backend.tmpDir, {
        fixture: comparisonTargetFixture,
        releaseBackendEnv,
      });
    } catch (setupError) {
      try {
        await releaseBackendEnv?.();
      } finally {
        await comparisonTargetFixture.close();
      }
      throw setupError;
    }
  } else {
    comparisonTargetURL = `file://${targetRemoteDir}`;
  }
  if (options.comparisonTargetAvailable !== false) {
    execFileSync("git", ["config", "--global", `url.${comparisonTargetURL}.insteadOf`, targetURL], {
      cwd: backend.tmpDir,
      env: gitEnv,
    });
  }

  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "github",
    provider_repo_id: "42",
    provider_host: "https://github.com",
    provider_owner: "contributor",
    provider_name: "widget-fork",
  });

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("contributor");
  await apiClient.mockGitHubAddPRs([
    {
      number: 1701,
      title: "Fork contribution",
      state: "open",
      head_branch: "feature/fork",
      head_sha: headSHA,
      base_branch: "main",
      author_login: "contributor",
      repo_owner: "upstream",
      repo_name: "widget",
      head_repo_id: 42,
      head_repo_owner: "contributor",
      head_repo_name: "widget-fork",
      head_repo_clone_url: "https://github.com/contributor/widget-fork.git",
      base_repo_id: 99,
      base_repo_owner: "upstream",
      base_repo_name: "widget",
      base_default_branch: "main",
      maintainer_can_modify: true,
      html_url: "https://github.com/upstream/widget/pull/1701",
    },
  ]);
  await apiClient.mockGitHubAddPRFiles("upstream", "widget", 1701, [
    { filename: "fork-one.txt", status: "added", additions: 1, deletions: 0 },
    { filename: "fork-two.txt", status: "added", additions: 1, deletions: 0 },
    { filename: "fork-three.txt", status: "added", additions: 1, deletions: 0 },
  ]);
  await apiClient.mockGitHubAddPRCommits("upstream", "widget", 1701, [
    {
      sha: headSHA,
      message: "Add three fork contribution files",
      author_login: "contributor",
      author_date: "2026-08-19T12:00:00Z",
      stats_available: true,
    },
  ]);

  const task = await apiClient.createTask(seedData.workspaceId, "Fork PR comparison target", {
    description: "/e2e:simple-message",
    agent_profile_id: seedData.agentProfileId,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repositories: [
      {
        repository_id: seedData.repositoryId,
        base_branch: "main",
        checkout_branch: "feature/fork",
      },
    ],
    executor_profile_id: comparisonExecutorProfileID,
  });

  await apiClient.associateGitHubTaskPR({
    workspace_id: seedData.workspaceId,
    task_id: task.id,
    repository_id: seedData.repositoryId,
    pr_url: "https://github.com/upstream/widget/pull/1701",
  });
  await apiClient.launchSession({
    task_id: task.id,
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: comparisonExecutorProfileID,
    workflow_step_id: seedData.startStepId,
    prompt: "/e2e:simple-message",
  });

  const active = activeComparisonTargetFixtures.get(backend.tmpDir);
  activeComparisonTargetFixtures.set(backend.tmpDir, {
    ...active,
    taskID: task.id,
  });

  return { task, headSHA, comparisonTargetFixture };
}

function gitConfigEnvVarsForURLRewrite(comparisonTargetURL: string) {
  return [
    { key: "GIT_CONFIG_KEY_0", value: `url.${comparisonTargetURL}.insteadOf` },
    { key: "GIT_CONFIG_VALUE_0", value: targetURL },
  ];
}

export async function resetForkPRComparisonRepository(
  seedData: SeedData,
  backend: BackendContext,
  apiClient: ApiClient,
) {
  const active = activeComparisonTargetFixtures.get(backend.tmpDir);
  activeComparisonTargetFixtures.delete(backend.tmpDir);
  await active?.releaseBackendEnv?.();
  await active?.fixture?.close();
  await clearTaskComparisonTargetRefs(active?.taskID, apiClient, backend.tmpDir);
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  clearComparisonTargetRefs(seedData, backend.tmpDir);
}

async function clearTaskComparisonTargetRefs(
  taskID: string | undefined,
  apiClient: ApiClient,
  tmpDir: string,
) {
  if (!taskID) return;
  const environment = await apiClient.getTaskEnvironment(taskID);
  const repositoryPaths = new Set<string>();
  if (environment?.workspace_path) repositoryPaths.add(environment.workspace_path);
  if (environment?.worktree_path) repositoryPaths.add(environment.worktree_path);
  for (const repository of environment?.repos ?? []) {
    if (repository.worktree_path) repositoryPaths.add(repository.worktree_path);
  }
  for (const repositoryPath of repositoryPaths) {
    clearComparisonTargetRefsAtPath(repositoryPath, tmpDir);
  }
}

function clearComparisonTargetRefs(seedData: SeedData, tmpDir: string) {
  clearComparisonTargetRefsAtPath(seedData.repositoryPath, tmpDir);
}

function clearComparisonTargetRefsAtPath(repositoryPath: string, tmpDir: string) {
  if (!fs.existsSync(repositoryPath)) return;
  const env = makeGitEnv(tmpDir);
  let output: string;
  try {
    output = execFileSync(
      "git",
      ["-C", repositoryPath, "for-each-ref", "--format=%(refname)", "refs/remotes/compare-*"],
      { env },
    )
      .toString()
      .trim();
  } catch {
    return;
  }
  for (const ref of output ? output.split(/\r?\n/) : []) {
    execFileSync("git", ["-C", repositoryPath, "update-ref", "-d", ref], { env });
  }
}

async function startComparisonTargetAuthFixture(
  repoDir: string,
): Promise<ComparisonTargetAuthFixture> {
  const repoName = path.basename(repoDir);
  const repoPath = `/${repoName}`;
  let available = false;
  let unauthorizedRequests = 0;
  const children = new Set<ChildProcess>();
  const requestContext: ComparisonTargetRequestContext = {
    repoDir,
    repoPath,
    isAvailable: () => available,
    recordUnauthorized: () => {
      unauthorizedRequests += 1;
    },
    children,
  };

  const server = createServer((request, response) => {
    void serveComparisonTargetRequest(request, response, requestContext);
  });
  const port = await listenComparisonTargetServer(server);

  return {
    url: `http://127.0.0.1:${port}${repoPath}`,
    unauthorizedRequestCount: () => unauthorizedRequests,
    setAvailable: (next) => {
      available = next;
    },
    close: async () => {
      for (const child of children) child.kill();
      await closeComparisonTargetServer(server);
    },
  };
}

type ComparisonTargetRequestContext = {
  repoDir: string;
  repoPath: string;
  isAvailable: () => boolean;
  recordUnauthorized: () => void;
  children: Set<ChildProcess>;
};

async function serveComparisonTargetRequest(
  request: IncomingMessage,
  response: ServerResponse,
  context: ComparisonTargetRequestContext,
): Promise<void> {
  const { repoDir, repoPath, isAvailable, recordUnauthorized, children } = context;
  const parsed = new URL(request.url ?? "/", "http://comparison-target");
  const pathname = decodeURIComponent(parsed.pathname);
  const isAdvertiseRequest = pathname === `${repoPath}/info/refs`;
  const isUploadRequest = pathname === `${repoPath}/git-upload-pack`;
  if (
    (!isAdvertiseRequest && !isUploadRequest) ||
    (isAdvertiseRequest && request.method !== "GET") ||
    (isUploadRequest && request.method !== "POST")
  ) {
    request.resume();
    response.writeHead(404).end();
    return;
  }

  const available = isAvailable();
  if (!available) {
    recordUnauthorized();
    request.resume();
    response.writeHead(401, { "www-authenticate": 'Basic realm="comparison-target"' }).end();
    return;
  }

  const child = spawn("git", ["http-backend"], {
    env: {
      ...process.env,
      GIT_HTTP_EXPORT_ALL: "1",
      GIT_PROJECT_ROOT: path.dirname(repoDir),
      PATH_INFO: pathname,
      QUERY_STRING: parsed.search.slice(1),
      REQUEST_METHOD: request.method ?? "GET",
      CONTENT_TYPE: String(request.headers["content-type"] ?? ""),
      CONTENT_LENGTH: String(request.headers["content-length"] ?? ""),
      ...(request.headers["git-protocol"]
        ? { HTTP_GIT_PROTOCOL: String(request.headers["git-protocol"]) }
        : {}),
    },
  });
  children.add(child);
  const output: Buffer[] = [];
  child.stdout.on("data", (chunk: Buffer) => output.push(chunk));
  child.stderr.resume();
  child.stdin.on("error", () => undefined);
  child.stdout.on("error", () => undefined);
  child.once("error", () => {
    children.delete(child);
    response.writeHead(500).end();
  });
  child.once("close", (code) => {
    children.delete(child);
    if (code !== 0) {
      response.writeHead(500).end();
      return;
    }
    const cgiResponse = Buffer.concat(output);
    const separator = cgiResponse.indexOf(Buffer.from("\r\n\r\n"));
    const separatorLength = separator >= 0 ? 4 : 2;
    const headerEnd = separator >= 0 ? separator : cgiResponse.indexOf(Buffer.from("\n\n"));
    if (headerEnd < 0) {
      response.writeHead(500).end();
      return;
    }
    const headers = cgiResponse.subarray(0, headerEnd).toString("utf8").split(/\r?\n/);
    let status = 200;
    const responseHeaders: Record<string, string> = {};
    for (const header of headers) {
      const separatorIndex = header.indexOf(":");
      if (separatorIndex < 0) continue;
      const name = header.slice(0, separatorIndex);
      const value = header.slice(separatorIndex + 1).trim();
      if (name.toLowerCase() === "status") {
        status = Number.parseInt(value, 10) || 500;
      } else {
        responseHeaders[name] = value;
      }
    }
    response.writeHead(status, responseHeaders);
    response.end(cgiResponse.subarray(headerEnd + separatorLength));
  });
  if (isAdvertiseRequest) child.stdin.end();
  else request.pipe(child.stdin);
}

function listenComparisonTargetServer(server: Server): Promise<number> {
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      server.off("error", reject);
      const address = server.address();
      if (!address || typeof address === "string") {
        reject(new Error("comparison target fixture did not receive a TCP port"));
        return;
      }
      resolve(address.port);
    });
  });
}

function closeComparisonTargetServer(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
    server.closeAllConnections();
  });
}
