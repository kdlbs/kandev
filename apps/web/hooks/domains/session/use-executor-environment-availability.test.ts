import { describe, expect, it } from "vitest";
import { isExecutorEnvironmentUnavailable } from "./use-executor-environment-availability";
import type { ExecutorEnvironmentStatus } from "@/components/task/executor-environment-status";

function status(label: string, tone: ExecutorEnvironmentStatus["tone"]) {
  return { label, tone };
}

describe("isExecutorEnvironmentUnavailable", () => {
  it("does not block before an environment exists or while it is starting", () => {
    expect(isExecutorEnvironmentUnavailable(null)).toBe(false);
    expect(isExecutorEnvironmentUnavailable(status("starting", "warn"))).toBe(false);
  });

  it("blocks when the environment is no longer usable", () => {
    expect(isExecutorEnvironmentUnavailable(status("exited (137)", "error"))).toBe(true);
    expect(isExecutorEnvironmentUnavailable(status("paused", "warn"))).toBe(true);
    expect(isExecutorEnvironmentUnavailable(status("missing", "warn"))).toBe(true);
  });
});
