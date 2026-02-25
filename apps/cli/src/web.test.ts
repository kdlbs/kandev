import { EventEmitter } from "node:events";
import { PassThrough } from "node:stream";
import type { ChildProcess } from "node:child_process";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const spawnMock = vi.fn();

vi.mock("node:child_process", () => ({
  spawn: (...args: unknown[]) => spawnMock(...args),
}));

import { launchWebApp } from "./web";

type Supervisor = {
  children: Array<{ pid?: number }>;
  shutdown: (reason: string) => Promise<void>;
  attachSignalHandlers: () => void;
};

class FakeChildProcess extends EventEmitter {
  pid = 12345;
  stderr = new PassThrough();
}

function createSupervisor(): Supervisor {
  return {
    children: [],
    shutdown: vi.fn().mockResolvedValue(undefined),
    attachSignalHandlers: vi.fn(),
  };
}

describe("launchWebApp", () => {
  beforeEach(() => {
    spawnMock.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("does not auto-open browser as a side effect", () => {
    const proc = new FakeChildProcess() as unknown as ChildProcess;
    spawnMock.mockReturnValue(proc);
    const supervisor = createSupervisor();

    const launched = launchWebApp({
      command: "node",
      args: ["server.js"],
      cwd: "/tmp",
      env: process.env,
      supervisor: supervisor as any,
      label: "web",
    });

    expect(launched).toBe(proc);
    expect(supervisor.children).toContain(proc);
    expect(spawnMock).toHaveBeenCalledTimes(1);
  });

  it("pipes stderr in quiet mode", () => {
    const proc = new FakeChildProcess() as unknown as ChildProcess;
    const pipeSpy = vi.spyOn(proc.stderr as PassThrough, "pipe");
    spawnMock.mockReturnValue(proc);
    const supervisor = createSupervisor();

    launchWebApp({
      command: "node",
      args: ["server.js"],
      cwd: "/tmp",
      env: process.env,
      supervisor: supervisor as any,
      label: "web",
      quiet: true,
    });

    expect(spawnMock).toHaveBeenCalledWith("node", ["server.js"], {
      cwd: "/tmp",
      env: process.env,
      stdio: ["ignore", "pipe", "pipe"],
    });
    expect(pipeSpy).toHaveBeenCalledWith(process.stderr);
  });

  it("shuts down supervisor when web process exits", async () => {
    const proc = new FakeChildProcess() as unknown as ChildProcess;
    spawnMock.mockReturnValue(proc);
    const supervisor = createSupervisor();
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    vi.spyOn(process, "exit").mockImplementation((() => undefined) as never);

    launchWebApp({
      command: "node",
      args: ["server.js"],
      cwd: "/tmp",
      env: process.env,
      supervisor: supervisor as any,
      label: "web",
    });

    proc.emit("exit", 2, null);
    await Promise.resolve();
    await Promise.resolve();

    expect(supervisor.shutdown).toHaveBeenCalledWith("web exit");
    expect(consoleErrorSpy).toHaveBeenCalledWith("[kandev] web exited (code=2, signal=null)");
  });
});
