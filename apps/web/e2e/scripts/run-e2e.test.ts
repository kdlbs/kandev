import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { afterEach, describe, expect, it } from "vitest";

const scriptPath = path.resolve(__dirname, "run-e2e.sh");
const tempDirs: string[] = [];

afterEach(() => {
  for (const dir of tempDirs.splice(0)) fs.rmSync(dir, { recursive: true, force: true });
});

describe("run-e2e.sh", () => {
  it("marks a managed containers run before invoking Playwright", () => {
    const binDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-runner-"));
    tempDirs.push(binDir);
    const pnpmPath = path.join(binDir, "pnpm");
    fs.writeFileSync(pnpmPath, "#!/usr/bin/env sh\nprintf '%s' \"${KANDEV_E2E_CONTAINERS:-}\"\n");
    fs.chmodSync(pnpmPath, 0o755);

    const result = spawnSync(
      "bash",
      [scriptPath, "--host", "--no-build", "--project", "containers", "--", "--help"],
      {
        encoding: "utf8",
        env: { ...process.env, PATH: `${binDir}:${process.env.PATH ?? ""}` },
      },
    );

    expect(result.status).toBe(0);
    expect(result.stdout).toBe("1");
  });
});
