import { spawn } from "node:child_process";
import { once } from "node:events";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, it } from "vitest";

function running(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

function cancellationHarness(directory: string): string {
  const modulePath = path.resolve(__dirname, "planned-processes.ts");
  const file = path.join(directory, "cancel.mts");
  fs.writeFileSync(
    file,
    `
import { spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
const { executeSubshards } = createRequire(import.meta.url)(${JSON.stringify(modulePath)});
const directory = ${JSON.stringify(directory)};
const units = ["first.spec.ts", "second.spec.ts", "third.spec.ts", "fourth.spec.ts"].map(file => ({
  id: file, file, project: "chromium", fileHash: "hash", testCount: 1,
  estimatedSeconds: 1, classification: "profile"
}));
const shard = { index: 1, files: units.map(unit => unit.file), units, predictedSeconds: 4 };
const children = [];
const launch = (_command, _args, options) => {
  const child = spawn(process.execPath, ["-e", "setInterval(() => {}, 1000)"], {
    ...options, stdio: "ignore"
  });
  children.push(child.pid);
  fs.writeFileSync(path.join(directory, "children.json"), JSON.stringify(children));
  const output = options.env.PLAYWRIGHT_BLOB_OUTPUT_DIR;
  fs.mkdirSync(output, { recursive: true });
  fs.writeFileSync(path.join(output, "report.zip"), "test report");
  return child;
};
const pending = executeSubshards({cohort: "normal", projects: ["chromium"], shardCount: 14}, shard, 4, { spawn: launch, outputDir: path.join(directory, "reports") });
fs.writeFileSync(path.join(directory, "ready"), "ready");
process.exitCode = await pending;
`,
  );
  return file;
}

it.skipIf(process.platform === "win32")(
  "cancellation joins every owned Unix process and records failure",
  async () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-shard-cancel-"));
    const child = spawn(process.execPath, ["--import", "tsx", cancellationHarness(directory)], {
      stdio: ["ignore", "pipe", "pipe"],
      env: { ...process.env, CI: "1" },
    });
    let diagnostics = "";
    child.stderr!.on("data", (data) => {
      diagnostics += data.toString();
    });
    const closed = once(child, "close");
    try {
      await expect
        .poll(() => fs.existsSync(path.join(directory, "ready")), { timeout: 10_000 })
        .toBe(true);
      const pids = JSON.parse(
        fs.readFileSync(path.join(directory, "children.json"), "utf8"),
      ) as number[];
      expect(pids).toHaveLength(4);
      expect(pids.every(running)).toBe(true);
      child.kill("SIGTERM");
      const [code] = await closed;
      expect(code, diagnostics).toBe(143);
      expect(pids.some(running)).toBe(false);
      const metadata = JSON.parse(
        fs.readFileSync(path.join(directory, "reports/shard-normal-1-metadata.json"), "utf8"),
      );
      expect(metadata.exitCode).toBe(143);
      expect(metadata.processCount).toBe(4);
    } catch (error) {
      throw new Error(diagnostics || "No child stderr", { cause: error });
    } finally {
      if (child.exitCode === null) child.kill("SIGKILL");
      await closed;
      const owned = path.join(directory, "children.json");
      if (fs.existsSync(owned)) {
        for (const pid of JSON.parse(fs.readFileSync(owned, "utf8"))) {
          try {
            process.kill(-pid, "SIGKILL");
          } catch {
            /* Already joined. */
          }
        }
      }
      fs.rmSync(directory, { recursive: true, force: true });
    }
  },
  15_000,
);
