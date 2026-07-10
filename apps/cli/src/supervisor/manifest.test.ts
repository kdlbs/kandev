import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { afterEach, describe, expect, it } from "vitest";
import {
  allowedEnv,
  buildLaunchManifest,
  readLaunchManifest,
  writeLaunchManifest,
} from "./manifest";

const tmpDirs: string[] = [];

afterEach(() => {
  for (const dir of tmpDirs.splice(0)) {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

describe("supervisor launch manifest", () => {
  it("writes structured launch data with an allowlisted env", () => {
    const home = tempDir();
    const manifest = buildLaunchManifest({
      backend_executable: "/bin/kandev",
      argv: ["--serve"],
      cwd: home,
      env: {
        KANDEV_HOME_DIR: home,
        KANDEV_SERVER_PORT: "38429",
        SECRET_TOKEN: "do-not-write",
      },
      home_dir: home,
      port: 38429,
      mode: "cli",
      now: new Date("2026-06-14T18:00:00Z"),
    });
    const target = path.join(home, "supervisor", "launch.json");

    writeLaunchManifest(manifest, target);

    expect(readLaunchManifest(target)).toEqual(manifest);
    expect(readLaunchManifest(target).env).toEqual({
      KANDEV_HOME_DIR: home,
      KANDEV_SERVER_PORT: "38429",
    });
    if (process.platform !== "win32") {
      expect((fs.statSync(target).mode & 0o777).toString(8)).toBe("600");
    }
  });

  it("rejects relative executable, cwd, and home paths", () => {
    const base = {
      backend_executable: "/bin/kandev",
      argv: [] as string[],
      cwd: "/tmp",
      env: {},
      home_dir: "/tmp/kandev",
      port: 38429,
      mode: "cli",
    };

    expect(() => buildLaunchManifest({ ...base, backend_executable: "kandev" })).toThrow(
      /backend_executable/,
    );
    expect(() => buildLaunchManifest({ ...base, cwd: "relative" })).toThrow(/cwd/);
    expect(() => buildLaunchManifest({ ...base, home_dir: "relative" })).toThrow(/home_dir/);
  });

  it("does not copy arbitrary environment variables", () => {
    expect(
      allowedEnv({
        KANDEV_HOME_DIR: "/tmp/kandev",
        AWS_SECRET_ACCESS_KEY: "secret",
        GITHUB_TOKEN: "secret",
      }),
    ).toEqual({ KANDEV_HOME_DIR: "/tmp/kandev" });
  });
});

function tempDir(): string {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-supervisor-"));
  tmpDirs.push(dir);
  return dir;
}
