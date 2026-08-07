import type { ChildProcess } from "node:child_process";
import { describe, expect, test, vi } from "vitest";
import { killProcessGroup } from "../../e2e/fixtures/backend";

function childProcessWithPid(pid: number): ChildProcess {
  return { pid } as ChildProcess;
}

describe("killProcessGroup", () => {
  test("uses taskkill for the full process tree on Windows", async () => {
    const killWindowsTree = vi.fn((_pid: number, done: (error?: Error) => void) => done());

    await killProcessGroup(childProcessWithPid(42), "win32", killWindowsTree);

    expect(killWindowsTree).toHaveBeenCalledOnce();
    expect(killWindowsTree).toHaveBeenCalledWith(42, expect.any(Function));
  });

  test("reports a taskkill failure while the process is still alive", async () => {
    const failure = new Error("taskkill failed");
    const killWindowsTree = (_pid: number, done: (error?: Error) => void) => done(failure);

    await expect(
      killProcessGroup(childProcessWithPid(42), "win32", killWindowsTree, () => true),
    ).rejects.toBe(failure);
  });

  test("accepts a taskkill race when the process has already exited", async () => {
    const killWindowsTree = (_pid: number, done: (error?: Error) => void) =>
      done(new Error("not found"));

    await expect(
      killProcessGroup(childProcessWithPid(42), "win32", killWindowsTree, () => false),
    ).resolves.toBeUndefined();
  });
});
