import { EventEmitter } from "node:events";
import type { ChildProcess } from "node:child_process";
import { afterEach, describe, expect, it, vi } from "vitest";
import { killProcessGroup } from "../fixtures/process-group";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("killProcessGroup", () => {
  it("waits for the process to exit after sending the hard-kill signal", async () => {
    vi.useFakeTimers();
    const proc = Object.assign(new EventEmitter(), { pid: 42, exitCode: null }) as ChildProcess;
    const signals: Array<string | number> = [];
    vi.spyOn(process, "kill").mockImplementation((_pid, signal) => {
      signals.push(signal ?? 0);
      return true;
    });

    let finished = false;
    const stopped = killProcessGroup(proc, "linux").then(() => {
      finished = true;
    });
    await vi.advanceTimersByTimeAsync(7_000);

    expect(signals).toEqual(["SIGTERM", "SIGKILL"]);
    expect(finished).toBe(false);

    proc.emit("exit", null, "SIGKILL");
    await stopped;

    expect(finished).toBe(true);
  });
});
