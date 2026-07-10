import { describe, expect, it, vi } from "vitest";

import {
  bootoutAndWait,
  bootstrapWithRetry,
  reloadService,
  type LaunchctlResult,
} from "./launchctl";

type Call = { args: string[]; stdio: string };

function recordingRun(statusFor: (call: Call, index: number) => number | null): {
  run: (args: string[], stdio: "inherit" | "ignore") => LaunchctlResult;
  calls: Call[];
} {
  const calls: Call[] = [];
  const run = (args: string[], stdio: "inherit" | "ignore"): LaunchctlResult => {
    const call = { args, stdio };
    const status = statusFor(call, calls.length);
    calls.push(call);
    return { status };
  };
  return { run, calls };
}

describe("bootoutAndWait", () => {
  it("polls `print` until the job is gone before returning", () => {
    // print returns 0 (still loaded) twice, then non-zero (gone).
    let printCount = 0;
    const { run, calls } = recordingRun((call) => {
      if (call.args[0] === "bootout") return 0;
      if (call.args[0] === "print") {
        printCount += 1;
        return printCount <= 2 ? 0 : 1;
      }
      return 0;
    });
    const sleep = vi.fn();

    bootoutAndWait("gui/501/com.kdlbs.kandev", { run, sleep });

    expect(calls[0].args).toEqual(["bootout", "gui/501/com.kdlbs.kandev"]);
    expect(calls.filter((c) => c.args[0] === "print")).toHaveLength(3);
    expect(sleep).toHaveBeenCalledTimes(2);
  });

  it("returns immediately when nothing is loaded", () => {
    const { run, calls } = recordingRun((call) => (call.args[0] === "print" ? 1 : 0));
    const sleep = vi.fn();

    bootoutAndWait("gui/501/com.kdlbs.kandev", { run, sleep });

    expect(calls.filter((c) => c.args[0] === "print")).toHaveLength(1);
    expect(sleep).not.toHaveBeenCalled();
  });
});

describe("bootstrapWithRetry", () => {
  it("retries through the transient EIO and succeeds", () => {
    // Reproduces the launchd teardown race: bootstrap returns 5 twice, then 0.
    let bootstrapCount = 0;
    const { run, calls } = recordingRun(() => {
      bootstrapCount += 1;
      return bootstrapCount < 3 ? 5 : 0;
    });
    const sleep = vi.fn();

    expect(() =>
      bootstrapWithRetry("gui/501", "/Users/a/Library/LaunchAgents/x.plist", { run, sleep }),
    ).not.toThrow();
    expect(calls).toHaveLength(3);
    expect(sleep).toHaveBeenCalledTimes(2);
  });

  it("throws with the last exit code after exhausting retries", () => {
    const { run, calls } = recordingRun(() => 5);
    const sleep = vi.fn();

    expect(() =>
      bootstrapWithRetry("gui/501", "/Users/a/Library/LaunchAgents/x.plist", { run, sleep }),
    ).toThrow(/failed with code 5/);
    expect(calls).toHaveLength(5);
  });

  it("succeeds on the first attempt without sleeping", () => {
    const { run, calls } = recordingRun(() => 0);
    const sleep = vi.fn();

    bootstrapWithRetry("gui/501", "/x.plist", { run, sleep });

    expect(calls).toHaveLength(1);
    expect(sleep).not.toHaveBeenCalled();
  });
});

describe("reloadService", () => {
  it("waits for teardown, then bootstraps — surviving an EIO mid-reload", () => {
    // Live-service reload: print stays loaded once, bootstrap hits EIO once.
    let printCount = 0;
    let bootstrapCount = 0;
    const { run, calls } = recordingRun((call) => {
      if (call.args[0] === "print") {
        printCount += 1;
        return printCount <= 1 ? 0 : 1;
      }
      if (call.args[0] === "bootstrap") {
        bootstrapCount += 1;
        return bootstrapCount < 2 ? 5 : 0;
      }
      return 0;
    });
    const sleep = vi.fn();

    reloadService("gui/501/com.kdlbs.kandev", "gui/501", "/x.plist", { run, sleep });

    const order = calls.map((c) => c.args[0]);
    expect(order[0]).toBe("bootout");
    // bootout precedes the first bootstrap
    expect(order.indexOf("bootout")).toBeLessThan(order.indexOf("bootstrap"));
    expect(calls.filter((c) => c.args[0] === "bootstrap")).toHaveLength(2);
  });
});
