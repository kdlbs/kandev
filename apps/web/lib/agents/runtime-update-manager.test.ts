import { describe, expect, it } from "vitest";

import type { AgentUpdateJob, AgentUpdateStatus, InstallJob } from "@/lib/api";
import {
  buildRuntimeUpdateManagerModel,
  defaultRuntimeSelection,
  reconcileRuntimeSelection,
  runtimeUpdateGroup,
  runtimePolicyKey,
} from "./runtime-update-manager";

function status(overrides: Partial<AgentUpdateStatus> = {}): AgentUpdateStatus {
  return {
    display_name: "Claude",
    runtime_id: "npm:@example/claude",
    agent_name: "claude-acp",
    owner: "kandev",
    mechanism: "npm_candidate",
    management: "managed",
    update_mode: "pinned",
    package: "@example/claude",
    source: "@example/claude",
    default_version: "1.0.0",
    effective_version: "1.0.0",
    current_version: "1.0.0",
    latest_version: "1.1.0",
    available: true,
    enabled: true,
    auto_update_supported: true,
    auto_update: false,
    check_state: "update_available",
    ...overrides,
  };
}

const control = () => true;

describe("runtime update manager view model", () => {
  it("counts only known individually actionable updates and keeps unknown separate", () => {
    const model = buildRuntimeUpdateManagerModel(
      [
        status(),
        status({
          agent_name: "codex",
          runtime_id: "npm:@example/codex",
          check_state: "unknown",
        }),
        status({
          agent_name: "cursor",
          runtime_id: "native:cursor",
          owner: "external",
          management: "manual",
          mechanism: "native",
          update_mode: undefined,
          check_state: "update_available",
          guidance_url: "https://example.com/cursor",
        }),
        status({
          agent_name: "custom",
          runtime_id: "custom",
          management: "unsupported",
          mechanism: "custom",
          check_state: "up_to_date",
        }),
        status({
          agent_name: "fallback",
          runtime_id: "native:@example/fallback",
          owner: "external",
          management: "manual",
          mechanism: "native",
          managed_fallback: true,
          update_mode: undefined,
        }),
      ],
      { hasRuntimeControl: control },
    );

    expect(model.availableCount).toBe(1);
    expect(model.groups.unknown.map((row) => row.status.agent_name)).toEqual(["codex"]);
    expect(model.groups.external.map((row) => row.status.agent_name)).toEqual([
      "cursor",
      "fallback",
    ]);
    expect(model.groups.unavailable.map((row) => row.status.agent_name)).toEqual(["custom"]);
    expect(model.bulkCandidates.map((row) => row.status.agent_name)).toEqual(["claude-acp"]);
  });

  it("excludes busy, dirty, fallback, harness and unsupported runtimes from bulk", () => {
    const activeUpdate: AgentUpdateJob = {
      job_id: "job-1",
      agent_name: "busy",
      status: "updating",
      started_at: "2026-01-01T00:00:00Z",
    };
    const activeInstall: InstallJob = {
      job_id: "install-1",
      agent_name: "installing",
      status: "running",
      started_at: "2026-01-01T00:00:00Z",
    };
    const statuses = [
      status(),
      status({ agent_name: "busy", runtime_id: "npm:busy" }),
      status({ agent_name: "installing", runtime_id: "npm:installing" }),
      status({ agent_name: "dirty", runtime_id: "npm:dirty" }),
      status({
        agent_name: "opencode-acp",
        runtime_id: "npm:opencode",
        update_mode: "self_update",
        owner: "external",
        management: "manual",
        mechanism: "native",
      }),
      status({
        agent_name: "fallback",
        managed_fallback: true,
        owner: "external",
        management: "manual",
        mechanism: "native",
      }),
    ];
    const model = buildRuntimeUpdateManagerModel(statuses, {
      hasRuntimeControl: control,
      installJobs: { installing: activeInstall },
      updateJobs: { busy: activeUpdate },
      dirtyPolicyKeys: new Set([runtimePolicyKey(statuses[3])]),
    });
    expect(model.bulkCandidates.map((row) => row.status.agent_name)).toEqual(["claude-acp"]);
    expect(model.individualOnlyCount).toBe(1);
  });
});

describe("runtime update grouping", () => {
  it("never folds an unknown check into up to date", () => {
    expect(runtimeUpdateGroup(status({ check_state: "unknown" }))).toBe("unknown");
    expect(runtimeUpdateGroup(status({ check_state: "up_to_date" }))).toBe("up_to_date");
    expect(runtimeUpdateGroup(status({ available: false }))).toBe("unavailable");
  });

  it("drops vanished selections and never silently adds new candidates", () => {
    const first = buildRuntimeUpdateManagerModel([status()], { hasRuntimeControl: control });
    const initial = defaultRuntimeSelection(first);
    const expanded = buildRuntimeUpdateManagerModel(
      [status(), status({ agent_name: "codex", runtime_id: "npm:codex" })],
      { hasRuntimeControl: control },
    );
    expect([...reconcileRuntimeSelection(initial, expanded)]).toEqual([runtimePolicyKey(status())]);
  });
});
