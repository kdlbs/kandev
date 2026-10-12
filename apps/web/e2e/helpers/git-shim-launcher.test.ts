import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, it } from "vitest";
import { writeGitShimLauncher } from "../fixtures/git-shim-launcher";

it("retains owned Git controls when the instance environment excludes fixture variables", () => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-git launcher-"));
  try {
    const script = path.join(root, "shim.mjs");
    fs.writeFileSync(
      script,
      "process.stdout.write(JSON.stringify({delay:process.env.KANDEV_E2E_GIT_DELAY_FILE,args:process.argv.slice(2)}))",
    );
    writeGitShimLauncher(root, script, { KANDEV_E2E_GIT_DELAY_FILE: "owned gate path" });
    const command = path.join(root, process.platform === "win32" ? "git.cmd" : "git");
    const result = spawnSync(command, ["diff", "--numstat"], {
      env: { PATH: process.env.PATH, SystemRoot: process.env.SystemRoot },
      encoding: "utf8",
      shell: process.platform === "win32",
    });
    expect(result.status).toBe(0);
    expect(JSON.parse(result.stdout)).toEqual({
      delay: "owned gate path",
      args: ["diff", "--numstat"],
    });
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});
