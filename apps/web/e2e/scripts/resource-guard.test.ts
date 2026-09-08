import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { afterEach, describe, expect, it } from "vitest";

const guardPath = path.resolve(__dirname, "resource-guard.sh");
const tempDirs: string[] = [];

afterEach(() => {
  for (const directory of tempDirs.splice(0)) {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

function runGuard(
  functionName: string,
  args: string[] = [],
  extraEnv: Record<string, string> = {},
) {
  return spawnSync(
    "bash",
    ["-c", 'source "$1"; shift; ' + functionName + ' "$@"', "resource-guard", guardPath, ...args],
    {
      encoding: "utf8",
      env: { ...process.env, KANDEV_E2E_ALLOW_UNSAFE_PARALLELISM: "", ...extraEnv },
    },
  );
}

function fixtureMemory(meminfo: string, cgroupFiles: Record<string, string>) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-resource-guard-"));
  tempDirs.push(root);
  const meminfoPath = path.join(root, "meminfo");
  const cgroupRoot = path.join(root, "cgroup");
  fs.writeFileSync(meminfoPath, meminfo);
  for (const [relativePath, content] of Object.entries(cgroupFiles)) {
    const target = path.join(cgroupRoot, relativePath);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, content);
  }
  return { meminfoPath, cgroupRoot };
}

describe("resource-guard.sh", () => {
  it.each([["-j", "2"], ["-j=2"], ["-j2"], ["--workers", "2"], ["--workers=2"]])(
    "rejects Playwright worker syntax %s",
    (...args: string[]) => {
      const result = runGuard("e2e_validate_playwright_args", args);
      expect(result.status).toBe(1);
      expect(`${result.stdout}\n${result.stderr}`).toContain("worker limit");
    },
  );

  it("accepts one worker in every supported syntax", () => {
    for (const args of [["-j", "1"], ["-j=1"], ["-j1"], ["--workers", "1"], ["--workers=1"]]) {
      expect(runGuard("e2e_validate_playwright_args", args).status).toBe(0);
    }
  });

  it("uses the lower cgroup v2 remaining memory instead of host memory", () => {
    const fixture = fixtureMemory("MemAvailable:   14680064 kB\n", {
      "memory.max": String(4 * 1024 * 1024 * 1024),
      "memory.current": String(3 * 1024 * 1024 * 1024),
    });

    expect(
      runGuard("e2e_available_memory_kib", [fixture.meminfoPath, fixture.cgroupRoot]).stdout.trim(),
    ).toBe(String(1024 * 1024));
    expect(
      runGuard("e2e_max_local_shards", [fixture.meminfoPath, fixture.cgroupRoot]).stdout.trim(),
    ).toBe("1");
  });

  it("uses the lower cgroup v1 remaining memory when v2 is unavailable", () => {
    const fixture = fixtureMemory("MemAvailable:   8388608 kB\n", {
      "memory/memory.limit_in_bytes": String(3 * 1024 * 1024 * 1024),
      "memory/memory.usage_in_bytes": String(2 * 1024 * 1024 * 1024),
    });

    expect(
      runGuard("e2e_available_memory_kib", [fixture.meminfoPath, fixture.cgroupRoot]).stdout.trim(),
    ).toBe(String(1024 * 1024));
    expect(
      runGuard("e2e_max_local_shards", [fixture.meminfoPath, fixture.cgroupRoot]).stdout.trim(),
    ).toBe("1");
  });

  it("defaults to one shard when memory cannot be measured", () => {
    expect(
      runGuard("e2e_max_local_shards", [
        "/path/that/does/not/exist/meminfo",
        "/path/that/does/not/exist/cgroup",
      ]).stdout.trim(),
    ).toBe("1");
  });
});
