import { describe, expect, it } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor } from "@/lib/types/http";
import {
  flattenExecutorProfiles,
  resolveAgentProfileLabel,
  resolveExecutorProfileLabel,
} from "./profile-lookup";

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";

function mkAgentProfile(id: string, label: string): AgentProfileOption {
  return { id, label, agent_id: "claude", agent_name: "Claude", cli_passthrough: false };
}

function mkExecutor(id: string, profileNames: [string, string][]): Executor {
  return {
    id,
    name: id,
    type: "local",
    status: "ready",
    is_system: false,
    profiles: profileNames.map(([profileId, name]) => ({
      id: profileId,
      executor_id: id,
      name,
      prepare_script: "",
      cleanup_script: "",
      created_at: FIXTURE_TIMESTAMP,
      updated_at: FIXTURE_TIMESTAMP,
    })),
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
  };
}

describe("resolveAgentProfileLabel", () => {
  const profiles = [mkAgentProfile("agent-1", "Claude, Sonnet")];

  it("returns the profile's label when it resolves", () => {
    expect(resolveAgentProfileLabel("agent-1", profiles, "Removed")).toBe("Claude, Sonnet");
  });

  it("returns the missing label when the id resolves to nothing (B9)", () => {
    expect(resolveAgentProfileLabel("agent-missing", profiles, "Removed")).toBe("Removed");
  });
});

describe("flattenExecutorProfiles", () => {
  it("flattens every executor's profiles into one list", () => {
    const executors = [
      mkExecutor("exec-1", [["p1", "worktree"]]),
      mkExecutor("exec-2", [["p2", "docker"]]),
    ];
    expect(flattenExecutorProfiles(executors).map((p) => p.id)).toEqual(["p1", "p2"]);
  });

  it("returns an empty list for executors with no profiles", () => {
    const executors: Executor[] = [{ ...mkExecutor("exec-1", []), profiles: undefined }];
    expect(flattenExecutorProfiles(executors)).toEqual([]);
  });
});

describe("resolveExecutorProfileLabel", () => {
  const executors = [mkExecutor("exec-1", [["profile-1", "worktree"]])];

  it("returns the executor profile's name when it resolves", () => {
    expect(resolveExecutorProfileLabel("profile-1", executors, "Removed")).toBe("worktree");
  });

  it("returns the missing label when the id resolves to nothing (B9)", () => {
    expect(resolveExecutorProfileLabel("profile-missing", executors, "Removed")).toBe("Removed");
  });
});
