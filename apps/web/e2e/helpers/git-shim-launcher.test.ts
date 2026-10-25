import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { expect, it } from "vitest";
import { writeGitShimLauncher } from "../fixtures/git-shim-launcher";

it.each([
  { name: "delay control without a command hint", hint: undefined, args: ["status"] },
  { name: "matching command", hint: "diff\n", args: ["diff", "--numstat"] },
  { name: "global options", hint: "diff\n", args: ["-C", ".", "status"] },
  { name: "empty command hint", hint: "", args: ["status"] },
  { name: "invalid command hint", hint: "diff;exit\n", args: ["status"] },
  { name: "push control", hint: "diff\n", args: ["status"], push: true },
])("loads instrumentation for $name", ({ hint, args, push }) => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-git-control-"));
  try {
    const script = path.join(root, "shim.mjs");
    fs.writeFileSync(
      script,
      "process.stdout.write(JSON.stringify({delay:process.env.KANDEV_E2E_GIT_DELAY_FILE,push:process.env.KANDEV_E2E_GITLAB_PUSH_FILE,args:process.argv.slice(2)}))",
    );
    const env = {
      KANDEV_E2E_ORIGINAL_PATH: process.env.PATH ?? "",
      KANDEV_E2E_GIT_DELAY_FILE: path.join(root, "delay"),
      KANDEV_E2E_GITLAB_PUSH_FILE: path.join(root, "push"),
    };
    writeGitShimLauncher(root, script, env);
    fs.writeFileSync(env.KANDEV_E2E_GIT_DELAY_FILE, "active");
    if (hint !== undefined) fs.writeFileSync(`${env.KANDEV_E2E_GIT_DELAY_FILE}.command`, hint);
    if (push) fs.writeFileSync(env.KANDEV_E2E_GITLAB_PUSH_FILE, "active");
    const result = spawnSync(
      path.join(root, process.platform === "win32" ? "git.cmd" : "git"),
      args,
      {
        env: { PATH: process.env.PATH, SystemRoot: process.env.SystemRoot },
        encoding: "utf8",
        shell: process.platform === "win32",
      },
    );
    expect(result.status, result.stderr).toBe(0);
    expect(JSON.parse(result.stdout)).toEqual({
      delay: env.KANDEV_E2E_GIT_DELAY_FILE,
      push: env.KANDEV_E2E_GITLAB_PUSH_FILE,
      args,
    });
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

it.skipIf(process.platform === "win32").each([false, true])(
  "runs ordinary Git directly with a command-scoped control: %s",
  (active) => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-git ' $ launcher-"));
    try {
      const realBin = path.join(root, "real bin");
      const shimDir = path.join(root, "shim bin");
      fs.mkdirSync(realBin);
      fs.mkdirSync(shimDir);
      const script = path.join(root, "inactive.mjs");
      fs.writeFileSync(script, 'throw new Error("inactive instrumentation was loaded");');
      fs.writeFileSync(
        path.join(realBin, "git"),
        `#!${process.execPath}\nprocess.stdout.write(JSON.stringify({args:process.argv.slice(2),cwd:process.cwd(),path:process.env.PATH,delay:process.env.KANDEV_E2E_GIT_DELAY_FILE,input:require('node:fs').readFileSync(0,'utf8')}));process.stderr.write('git stderr');process.exit(17);\n`,
        { mode: 0o755 },
      );
      const delay = path.join(root, "absent delay");
      writeGitShimLauncher(shimDir, script, {
        KANDEV_E2E_ORIGINAL_PATH: realBin,
        KANDEV_E2E_GIT_DELAY_FILE: delay,
        KANDEV_E2E_GITLAB_PUSH_FILE: path.join(root, "absent push"),
      });
      if (active) {
        fs.writeFileSync(delay, JSON.stringify({ subcommand: "diff" }));
        fs.writeFileSync(`${delay}.command`, "diff\n");
      }
      const args = [
        ...(active ? ["status", "--porcelain=v2"] : ["-C", "folder with spaces", "diff"]),
        "--",
        "a'b",
        "$(literal)",
        "",
      ];
      const result = spawnSync(path.join(shimDir, "git"), args, {
        cwd: root,
        env: { PATH: shimDir },
        encoding: "utf8",
        input: "git stdin",
      });
      expect(result.status, result.stderr).toBe(17);
      expect(result.stderr).toBe("git stderr");
      expect(JSON.parse(result.stdout)).toEqual({
        args,
        cwd: root,
        path: realBin,
        delay,
        input: "git stdin",
      });
    } finally {
      fs.rmSync(root, { recursive: true, force: true });
    }
  },
);

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
