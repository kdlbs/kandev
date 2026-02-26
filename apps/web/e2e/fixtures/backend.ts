import { test as base } from "@playwright/test";
import { type ChildProcess, spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

const BACKEND_DIR = path.resolve(__dirname, "../../../../apps/backend");
const WEB_DIR = path.resolve(__dirname, "../..");
const KANDEV_BIN = path.join(BACKEND_DIR, "bin", "kandev");
const STANDALONE_SERVER = path.join(WEB_DIR, ".next/standalone/web/server.js");
const BACKEND_BASE_PORT = 18080;
const FRONTEND_BASE_PORT = 13000;
const HEALTH_TIMEOUT_MS = 30_000;
const HEALTH_POLL_MS = 250;

export type BackendContext = {
  port: number;
  baseUrl: string;
  frontendPort: number;
  frontendUrl: string;
};

async function waitForHealth(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {
      // not ready yet
    }
    await new Promise((r) => setTimeout(r, HEALTH_POLL_MS));
  }
  throw new Error(`Service did not become healthy at ${url} within ${timeoutMs}ms`);
}

function killProcess(proc: ChildProcess): Promise<void> {
  return new Promise<void>((resolve) => {
    proc.kill("SIGTERM");
    const timeout = setTimeout(() => {
      proc.kill("SIGKILL");
      resolve();
    }, 5_000);
    proc.on("exit", () => {
      clearTimeout(timeout);
      resolve();
    });
  });
}

/**
 * Worker-scoped fixture that spawns an isolated backend process and
 * a dedicated Next.js frontend. Each Playwright worker gets its own
 * backend on a unique port with an isolated HOME, database, and data
 * directory, plus its own frontend with SSR routed to that backend.
 */
export const backendFixture = base.extend<object, { backend: BackendContext }>({
  backend: [
    async ({}, use, workerInfo) => {
      const backendPort = BACKEND_BASE_PORT + workerInfo.workerIndex;
      const frontendPort = FRONTEND_BASE_PORT + workerInfo.workerIndex;
      const tmpDir = fs.mkdtempSync(
        path.join(os.tmpdir(), `kandev-e2e-${workerInfo.workerIndex}-`),
      );
      const dataDir = path.join(tmpDir, ".kandev");
      const dbPath = path.join(tmpDir, "kandev.db");
      const worktreeBase = path.join(tmpDir, "worktrees");
      const repoCloneBase = path.join(tmpDir, "repos");

      fs.mkdirSync(dataDir, { recursive: true });
      fs.mkdirSync(worktreeBase, { recursive: true });
      fs.mkdirSync(repoCloneBase, { recursive: true });

      // Write a minimal .gitconfig so git doesn't prompt for identity
      fs.writeFileSync(
        path.join(tmpDir, ".gitconfig"),
        "[user]\n  name = E2E Test\n  email = e2e@test.local\n",
      );

      const backendEnv = {
        ...stripGitHubTokens(process.env as Record<string, string>),
        HOME: tmpDir,
        KANDEV_DATA_DIR: dataDir,
        KANDEV_SERVER_PORT: String(backendPort),
        KANDEV_DATABASE_PATH: dbPath,
        KANDEV_MOCK_AGENT: "true",
        KANDEV_MOCK_GITHUB: "true",
        KANDEV_DOCKER_ENABLED: "false",
        KANDEV_WORKTREE_ENABLED: "false",
        KANDEV_WORKTREE_BASEPATH: worktreeBase,
        KANDEV_REPOCLONE_BASEPATH: repoCloneBase,
        KANDEV_LOG_LEVEL: "warn",
        GIT_AUTHOR_NAME: "E2E Test",
        GIT_AUTHOR_EMAIL: "e2e@test.local",
        GIT_COMMITTER_NAME: "E2E Test",
        GIT_COMMITTER_EMAIL: "e2e@test.local",
      };

      const debug = !!process.env.E2E_DEBUG;

      // --- Spawn backend ---
      const backendProc: ChildProcess = spawn(KANDEV_BIN, [], {
        env: backendEnv as unknown as NodeJS.ProcessEnv,
        stdio: ["ignore", "pipe", "pipe"],
      });

      backendProc.stderr?.on("data", (chunk: Buffer) => {
        if (debug) process.stderr.write(`[backend:${backendPort}] ${chunk.toString()}`);
      });

      const baseUrl = `http://localhost:${backendPort}`;
      await waitForHealth(`${baseUrl}/health`, HEALTH_TIMEOUT_MS);

      // --- Spawn frontend (Next.js standalone server) ---
      const frontendProc: ChildProcess = spawn("node", [STANDALONE_SERVER], {
        cwd: WEB_DIR,
        env: {
          ...(process.env as unknown as Record<string, string>),
          KANDEV_API_BASE_URL: baseUrl,
          PORT: String(frontendPort),
          HOSTNAME: "localhost",
          NODE_ENV: "production",
        } as unknown as NodeJS.ProcessEnv,
        stdio: ["ignore", "pipe", "pipe"],
      });

      frontendProc.stderr?.on("data", (chunk: Buffer) => {
        if (debug) process.stderr.write(`[frontend:${frontendPort}] ${chunk.toString()}`);
      });

      const frontendUrl = `http://localhost:${frontendPort}`;
      await waitForHealth(frontendUrl, HEALTH_TIMEOUT_MS);

      try {
        await use({ port: backendPort, baseUrl, frontendPort, frontendUrl });
      } finally {
        // Shutdown frontend first, then backend
        await killProcess(frontendProc);
        await killProcess(backendProc);

        // Cleanup temp directory
        fs.rmSync(tmpDir, { recursive: true, force: true });
      }
    },
    { scope: "worker", timeout: 60_000 },
  ],
});

/** Strip GH_TOKEN / GITHUB_TOKEN so the mock client is used. */
function stripGitHubTokens(env: Record<string, string>): Record<string, string> {
  const cleaned = { ...env };
  delete cleaned.GH_TOKEN;
  delete cleaned.GITHUB_TOKEN;
  return cleaned;
}
