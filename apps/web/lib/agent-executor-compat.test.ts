import { describe, expect, it } from "vitest";
import {
  executorRequiresAgentCredentials,
  shouldFilterHandoffByHostHealth,
} from "./agent-executor-compat";
import type { ExecutorType } from "@/lib/types/http";

describe("shouldFilterHandoffByHostHealth", () => {
  it.each(["local", "local_pc", "worktree"])("returns true for %s", (executor_type) => {
    expect(shouldFilterHandoffByHostHealth({ executor_type: executor_type as ExecutorType })).toBe(
      true,
    );
  });

  it.each(["local_docker", "remote_docker", "ssh", "remote_vps", "k8s"])(
    "returns false for %s",
    (executor_type) => {
      expect(
        shouldFilterHandoffByHostHealth({ executor_type: executor_type as ExecutorType }),
      ).toBe(false);
    },
  );

  it("does not apply a host-health filter while the executor is unknown", () => {
    expect(shouldFilterHandoffByHostHealth(null)).toBe(false);
  });

  it("requires remote agent credentials for Kubernetes profiles", () => {
    expect(executorRequiresAgentCredentials("k8s")).toBe(true);
  });
});
