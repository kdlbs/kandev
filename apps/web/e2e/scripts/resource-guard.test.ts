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

  it("resolves the current unified subgroup before checking remaining memory", () => {
    const fixture = fixtureMemory("MemAvailable: 16777216 kB\n", {
      "memory.max": String(16 * 1024 ** 3),
      "memory.current": "0",
      "worker/memory.max": String(4 * 1024 ** 3),
      "worker/memory.current": String(3 * 1024 ** 3),
    });
    const membership = path.join(path.dirname(fixture.meminfoPath), "membership");
    fs.writeFileSync(membership, "0::/worker\n");
    const resolved = runGuard("e2e_resolve_cgroup_root", [fixture.cgroupRoot, membership]);
    expect(resolved.status).toBe(0);
    expect(resolved.stdout.trim()).toBe(path.join(fixture.cgroupRoot, "worker"));
    expect(
      runGuard("e2e_available_memory_kib", [
        fixture.meminfoPath,
        resolved.stdout.trim(),
      ]).stdout.trim(),
    ).toBe("1048576");
  });

  it("retains the mounted namespace root and v1 fallback", () => {
    const fixture = fixtureMemory("MemAvailable: 16777216 kB\n", {
      "memory.max": String(4 * 1024 ** 3),
      "memory.current": "0",
    });
    const membership = path.join(path.dirname(fixture.meminfoPath), "membership");
    for (const scope of ["0::/\n", "3:memory:/worker\n"]) {
      fs.writeFileSync(membership, scope);
      expect(
        runGuard("e2e_resolve_cgroup_root", [fixture.cgroupRoot, membership])
          .stdout.trim()
          .replace(/\/$/, ""),
      ).toBe(fixture.cgroupRoot);
    }
  });

  it.each([
    [4, "1"],
    [6, "2"],
    [8, "3"],
    [10, "4"],
    [16, "4"],
  ])("reserves two GiB before budgeting planned stacks at %i GiB", (gib, expected) => {
    const fixture = fixtureMemory(`MemAvailable: ${Number(gib) * 1024 * 1024} kB\n`, {});
    const result = runGuard("e2e_max_planned_processes", [fixture.meminfoPath, fixture.cgroupRoot]);
    expect(result.status).toBe(0);
    expect(result.stdout.trim()).toBe(expected);
  });

  it("keeps planned processes within lower cgroup remaining memory", () => {
    const fixture = fixtureMemory("MemAvailable: 16777216 kB\n", {
      "memory.max": String(12 * 1024 ** 3),
      "memory.current": String(5 * 1024 ** 3),
    });
    expect(
      runGuard("e2e_max_planned_processes", [
        fixture.meminfoPath,
        fixture.cgroupRoot,
      ]).stdout.trim(),
    ).toBe("2");
  });

  it("retains managed local limits while allowing a separately measured planned budget", () => {
    const fixture = fixtureMemory("MemAvailable: 10485760 kB\n", {});
    expect(
      runGuard("e2e_max_local_shards", [fixture.meminfoPath, fixture.cgroupRoot]).stdout.trim(),
    ).toBe("2");
    expect(
      runGuard("e2e_max_planned_processes", [
        fixture.meminfoPath,
        fixture.cgroupRoot,
      ]).stdout.trim(),
    ).toBe("4");
  });

  it("keeps an unknown planned memory budget at one process", () => {
    expect(
      runGuard("e2e_max_planned_processes", ["/missing/meminfo", "/missing/cgroup"]).stdout.trim(),
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
