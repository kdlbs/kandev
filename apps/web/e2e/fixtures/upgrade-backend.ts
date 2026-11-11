import { test as base, expect, type TestInfo } from "@playwright/test";
import { spawn, execFileSync, type ChildProcess } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { once } from "node:events";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import net from "node:net";
import { assertUpgradeCI } from "../../../../scripts/upgrade/ci-only.mjs";
import { ApiClient } from "../helpers/api-client";
import { postgresSQL, prepareUpgradeDatabase } from "../helpers/upgrade-database";

const repoRoot = path.resolve(__dirname, "../../../..");
const digest = (file: string) => createHash("sha256").update(fs.readFileSync(file)).digest("hex");
export type ReleaseManifest = {
  tag: string;
  source_commit: string;
  archive_sha256: string;
  binary: string;
  binary_sha256: string;
  agentctl: string;
  agentctl_sha256: string;
  mock_agent: string;
  mock_agent_sha256: string;
};
type OwnedUpgrade = {
  startBaseline: () => Promise<void>;
  stop: () => Promise<void>;
  dispose: () => Promise<void>;
};
export async function withOwnedUpgrade(owner: OwnedUpgrade, use: () => Promise<void>) {
  try {
    await owner.startBaseline();
    await use();
  } finally {
    try {
      await owner.stop();
    } finally {
      await owner.dispose();
    }
  }
}
export function validateUpgradeInput(
  manifest: string | undefined,
  engine: string,
  postgres: string | undefined,
) {
  if (!manifest)
    throw new Error("upgrade manifest is required; run scripts/upgrade/prepare-release.mjs");
  if (!["sqlite", "postgres"].includes(engine))
    throw new Error(`unsupported upgrade engine: ${engine}`);
  if (engine === "postgres" && !postgres) throw new Error("PostgreSQL test service is required");
}
async function freePort() {
  const server = net.createServer();
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  const port = (server.address() as net.AddressInfo).port;
  await new Promise<void>((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
  return port;
}
export class UpgradeBackend {
  readonly root: string;
  readonly home: string;
  readonly database: string;
  readonly api: ApiClient;
  readonly manifest: ReleaseManifest;
  readonly env: Record<string, string>;
  readonly baselineDir: string;
  readonly candidate = process.env.KANDEV_E2E_BIN ?? path.join(repoRoot, "apps/backend/bin/kandev");
  readonly engine = process.env.KANDEV_UPGRADE_ENGINE ?? "sqlite";
  readonly postgresName = `upgrade_${randomUUID().replaceAll("-", "")}`;
  readonly evidence: Record<string, unknown> = { seed_version: 1, phases: [] };
  private proc?: ChildProcess;
  private baselineSealed = false;
  private phase = "created";
  private dbCreated = false;
  constructor(
    readonly baseUrl: string,
    readonly info: TestInfo,
  ) {
    assertUpgradeCI();
    const manifestPath = process.env.KANDEV_UPGRADE_MANIFEST;
    validateUpgradeInput(manifestPath, this.engine, process.env.KANDEV_UPGRADE_POSTGRES_URL);
    this.baselineDir = path.dirname(path.resolve(manifestPath!));
    this.manifest = JSON.parse(fs.readFileSync(manifestPath!, "utf8"));
    for (const [file, hash] of [
      [this.manifest.binary, this.manifest.binary_sha256],
      [this.manifest.agentctl, this.manifest.agentctl_sha256],
      [this.manifest.mock_agent, this.manifest.mock_agent_sha256],
    ]) {
      const resolved = path.resolve(this.baselineDir, file);
      if (!resolved.startsWith(this.baselineDir + path.sep) || digest(resolved) !== hash)
        throw new Error(`baseline artifact identity mismatch: ${file}`);
    }
    const candidateIdentity = {
      commit: execFileSync("git", ["rev-parse", "HEAD"], {
        cwd: repoRoot,
        encoding: "utf8",
      }).trim(),
      binary_sha256: digest(this.candidate),
      agentctl_sha256: digest(path.join(path.dirname(this.candidate), "agentctl")),
      mock_agent_sha256: digest(path.join(path.dirname(this.candidate), "mock-agent")),
    };
    this.root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-upgrade-"));
    this.home = path.join(this.root, ".kandev");
    this.database = path.join(this.root, "kandev.db");
    this.api = new ApiClient(baseUrl);
    this.env = {
      PATH: process.env.PATH ?? "/usr/bin:/bin",
      HOME: this.root,
      TMPDIR: this.root,
      KANDEV_HOME_DIR: this.home,
      KANDEV_DATABASE_PATH: this.database,
      KANDEV_SERVER_HOST: "127.0.0.1",
      KANDEV_SERVER_PORT: new URL(baseUrl).port,
      KANDEV_MOCK_AGENT: "only",
      KANDEV_MOCK_AGENT_MCP: "false",
      KANDEV_MOCK_GITHUB: "true",
      KANDEV_MOCK_GITLAB: "true",
      KANDEV_MOCK_JIRA: "true",
      KANDEV_MOCK_LINEAR: "true",
      KANDEV_MOCK_SENTRY: "true",
      KANDEV_MOCK_AZURE_DEVOPS: "true",
      KANDEV_FEATURES_AGENT_SURVIVAL: "false",
      KANDEV_DOCKER_ENABLED: "false",
      KANDEV_WORKTREE_ENABLED: "true",
      KANDEV_WORKTREE_BASEPATH: path.join(this.root, "worktrees"),
      KANDEV_REPOCLONE_BASEPATH: path.join(this.root, "clones"),
      AGENTCTL_AUTO_APPROVE_PERMISSIONS: "true",
      KANDEV_LOG_LEVEL: "info",
      GIT_AUTHOR_NAME: "Upgrade Test",
      GIT_AUTHOR_EMAIL: "upgrade@example.test",
      GIT_COMMITTER_NAME: "Upgrade Test",
      GIT_COMMITTER_EMAIL: "upgrade@example.test",
    };
    this.evidence.baseline = this.manifest;
    this.evidence.candidate = candidateIdentity;
    this.evidence.workflow = {
      ref: process.env.GITHUB_WORKFLOW_REF,
      sha: process.env.GITHUB_WORKFLOW_SHA,
    };
    this.evidence.engine = this.engine;
  }
  async initialize() {
    fs.mkdirSync(this.home);
    fs.writeFileSync(
      path.join(this.root, ".gitconfig"),
      "[user]\nname = Upgrade Test\nemail = upgrade@example.test\n[commit]\ngpgsign = false\n[tag]\ngpgsign = false\n",
    );
    if (this.engine === "postgres") {
      Object.assign(this.env, prepareUpgradeDatabase(this.postgresName));
      this.dbCreated = true;
      this.evidence.database_version = postgresSQL(this.postgresName, "SHOW server_version;");
    }
    // Reserve a disjoint agentctl range for this owned runtime.
    const agentPort = await freePort();
    Object.assign(this.env, {
      AGENTCTL_PORT: String(agentPort),
      AGENTCTL_INSTANCE_PORT_BASE: String(agentPort + 1),
      AGENTCTL_INSTANCE_PORT_MAX: String(Math.min(agentPort + 100, 65535)),
    });
    this.evidence.configuration = {
      ...this.env,
      KANDEV_DATABASE_PASSWORD: this.env.KANDEV_DATABASE_PASSWORD ? "<synthetic>" : undefined,
    };
    await this.start("baseline");
  }
  sealBaseline() {
    this.baselineSealed = true;
  }
  async start(which: "baseline" | "candidate") {
    if (which === "candidate" && !this.baselineSealed)
      throw new Error("candidate cannot initialize storage before baseline seed");
    await this.stop();
    this.phase = `${which}-boot`;
    const binary =
      which === "baseline" ? path.resolve(this.baselineDir, this.manifest.binary) : this.candidate;
    const env = {
      ...this.env,
      PATH: [path.dirname(binary), this.env.PATH].join(path.delimiter),
      ...(which === "candidate"
        ? { KANDEV_WEB_DIST_DIR: path.join(repoRoot, "apps/web/dist") }
        : {}),
    };
    const fd = fs.openSync(this.info.outputPath(`${which}-process.log`), "a");
    try {
      this.proc = spawn(binary, ["__backend"], {
        cwd: this.root,
        env,
        detached: true,
        stdio: ["ignore", fd, fd],
      });
    } finally {
      fs.closeSync(fd);
    }
    let spawnError: Error | undefined;
    this.proc.on("error", (error) => {
      spawnError = error;
    });
    await expect
      .poll(
        async () => {
          if (spawnError) throw spawnError;
          if (this.proc?.exitCode !== null)
            throw new Error(`${which} exited: ${this.proc?.exitCode}; see process log`);
          try {
            return (await fetch(`${this.baseUrl}/ready`, { signal: AbortSignal.timeout(2000) }))
              .status;
          } catch {
            return 0;
          }
        },
        { timeout: 90_000, message: `${which} readiness` },
      )
      .toBe(200);
    const health = await (await fetch(`${this.baseUrl}/health`)).json();
    this.evidence[`${which}_health`] = health;
    if (which === "baseline")
      expect(JSON.stringify(health)).toContain(this.manifest.tag.replace(/^v/, ""));
    else {
      const response = await fetch(`${this.baseUrl}/api/v1/system/info`);
      expect(response.ok, "candidate build information is available").toBe(true);
      const buildInfo = await response.json();
      expect(buildInfo.commit).toBe((this.evidence.candidate as { commit: string }).commit);
      this.evidence.candidate_build_info = buildInfo;
    }
    (this.evidence.phases as unknown[]).push({ phase: this.phase, at: new Date().toISOString() });
  }
  async stop(crash = false) {
    const proc = this.proc;
    if (!proc?.pid) return;
    const exited = once(proc, "exit");
    if (proc.exitCode === null && proc.signalCode === null) {
      try {
        process.kill(-proc.pid, crash ? "SIGKILL" : "SIGTERM");
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ESRCH") throw error;
      }
      const timer = setTimeout(() => {
        try {
          process.kill(-proc.pid!, "SIGKILL");
        } catch {}
      }, 7000);
      try {
        await exited;
      } finally {
        clearTimeout(timer);
      }
    }
    // Agentctl can outlive its parent's exit. Kill the complete owned group.
    try {
      process.kill(-proc.pid, "SIGKILL");
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "ESRCH") throw error;
    }
    this.proc = undefined;
  }
  async dispose() {
    this.evidence.last_phase = this.phase;
    fs.writeFileSync(
      this.info.outputPath("upgrade-manifest.json"),
      JSON.stringify(this.evidence, null, 2),
    );
    for (const entry of [
      "seed.json",
      "baseline-state.json",
      "candidate-state.json",
      "post-action.json",
    ]) {
      if (fs.existsSync(path.join(this.root, entry)))
        fs.copyFileSync(path.join(this.root, entry), this.info.outputPath(entry));
    }
    if (fs.existsSync(path.join(this.home, "logs")))
      fs.cpSync(path.join(this.home, "logs"), this.info.outputPath("runtime-logs"), {
        recursive: true,
      });
    if (this.dbCreated)
      postgresSQL("postgres", `DROP DATABASE "${this.postgresName}" WITH (FORCE);`);
    if (!this.root.startsWith(path.join(os.tmpdir(), "kandev-upgrade-")))
      throw new Error("refusing unowned cleanup");
    fs.rmSync(this.root, { recursive: true, force: true });
  }
}
export const test = base.extend<{ upgrade: UpgradeBackend }>({
  upgrade: async ({ browserName: _browserName }, use, info) => {
    const upgrade = new UpgradeBackend(`http://127.0.0.1:${await freePort()}`, info);
    await withOwnedUpgrade(
      {
        startBaseline: () => upgrade.initialize(),
        stop: () => upgrade.stop(),
        dispose: () => upgrade.dispose(),
      },
      () => use(upgrade),
    );
  },
});
export { expect };
