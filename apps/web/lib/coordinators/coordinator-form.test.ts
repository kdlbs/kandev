import { describe, expect, it } from "vitest";
import type { Coordinator, PatchCoordinatorRequest } from "@/lib/api/domains/coordinator-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import {
  buildCreateCoordinatorPayload,
  buildPatchCoordinatorPayload,
  coordinatorFormFromRecord,
  resolveDefaultAgentProfileId,
  resolveDefaultExecutorProfileId,
} from "./coordinator-form";

const FIXTURE_TIMESTAMP = "2026-01-01T00:00:00Z";
const SOME_CONTEXT = "Some context";

function mkAgentProfile(overrides: Partial<AgentProfileOption> = {}): AgentProfileOption {
  return {
    id: "agent-1",
    label: "Claude, Sonnet",
    agent_id: "claude",
    agent_name: "Claude",
    cli_passthrough: false,
    ...overrides,
  };
}

function mkExecutorProfile(overrides: Partial<ExecutorProfile> = {}): ExecutorProfile {
  return {
    id: "profile-1",
    executor_id: "exec-1",
    name: "worktree",
    prepare_script: "",
    cleanup_script: "",
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
    ...overrides,
  };
}

function mkExecutor(overrides: Partial<Executor> = {}): Executor {
  return {
    id: "exec-1",
    name: "Local",
    type: "local",
    status: "ready",
    is_system: false,
    profiles: [mkExecutorProfile()],
    created_at: FIXTURE_TIMESTAMP,
    updated_at: FIXTURE_TIMESTAMP,
    ...overrides,
  };
}

const TASK_AGENT = "task-agent-1";
const TASK_PROFILE = "task-profile-1";

describe("coordinatorFormFromRecord", () => {
  it("maps a Coordinator DTO to editable form fields", () => {
    const coordinator: Coordinator = {
      id: "c-1",
      workspace_id: "ws-1",
      name: "Planner",
      agent_profile_id: "agent-1",
      executor_profile_id: "profile-1",
      task_agent_profile_id: TASK_AGENT,
      task_executor_profile_id: TASK_PROFILE,
      context: SOME_CONTEXT,
      conversation_task_id: null,
      created_at: FIXTURE_TIMESTAMP,
      updated_at: FIXTURE_TIMESTAMP,
    };
    expect(coordinatorFormFromRecord(coordinator)).toEqual({
      name: "Planner",
      agentProfileId: "agent-1",
      executorProfileId: "profile-1",
      taskAgentProfileId: TASK_AGENT,
      taskExecutorProfileId: TASK_PROFILE,
      context: SOME_CONTEXT,
    });
  });
});

describe("buildCreateCoordinatorPayload", () => {
  it("trims the name and passes profile ids and context through", () => {
    const payload = buildCreateCoordinatorPayload({
      name: "  Planner  ",
      agentProfileId: "agent-1",
      executorProfileId: "profile-1",
      taskAgentProfileId: TASK_AGENT,
      taskExecutorProfileId: TASK_PROFILE,
      context: SOME_CONTEXT,
    });
    expect(payload).toEqual({
      name: "Planner",
      agent_profile_id: "agent-1",
      executor_profile_id: "profile-1",
      task_agent_profile_id: TASK_AGENT,
      task_executor_profile_id: TASK_PROFILE,
      context: SOME_CONTEXT,
    });
  });
});

describe("buildPatchCoordinatorPayload", () => {
  const saved = {
    name: "Planner",
    agentProfileId: "agent-1",
    executorProfileId: "profile-1",
    taskAgentProfileId: TASK_AGENT,
    taskExecutorProfileId: TASK_PROFILE,
    context: SOME_CONTEXT,
  };

  it("returns an empty patch when nothing changed", () => {
    expect(buildPatchCoordinatorPayload(saved, saved)).toEqual({});
  });

  it("includes only the changed fields, trimming the name", () => {
    const patch: PatchCoordinatorRequest = buildPatchCoordinatorPayload(
      { ...saved, name: "  New name  " },
      saved,
    );
    expect(patch).toEqual({ name: "New name" });
  });

  it("omits the name when only whitespace was added around the saved value", () => {
    expect(buildPatchCoordinatorPayload({ ...saved, name: `  ${saved.name}  ` }, saved)).toEqual(
      {},
    );
  });

  it("includes agent, executor and context independently", () => {
    expect(
      buildPatchCoordinatorPayload(
        { ...saved, agentProfileId: "agent-2", executorProfileId: "profile-2", context: "New" },
        saved,
      ),
    ).toEqual({
      agent_profile_id: "agent-2",
      executor_profile_id: "profile-2",
      context: "New",
    });
  });
});

describe("resolveDefaultAgentProfileId", () => {
  it("returns the workspace default when it exists and is not passthrough", () => {
    const profiles = [mkAgentProfile({ id: "agent-1", cli_passthrough: false })];
    expect(resolveDefaultAgentProfileId("agent-1", profiles)).toBe("agent-1");
  });

  it("returns empty when the default id is not set", () => {
    expect(resolveDefaultAgentProfileId(null, [mkAgentProfile()])).toBe("");
    expect(resolveDefaultAgentProfileId(undefined, [mkAgentProfile()])).toBe("");
  });

  it("returns empty when the default profile no longer exists", () => {
    expect(resolveDefaultAgentProfileId("agent-missing", [mkAgentProfile({ id: "agent-1" })])).toBe(
      "",
    );
  });

  it("returns empty when the default profile is CLI-passthrough", () => {
    const profiles = [mkAgentProfile({ id: "agent-1", cli_passthrough: true })];
    expect(resolveDefaultAgentProfileId("agent-1", profiles)).toBe("");
  });
});

describe("resolveDefaultExecutorProfileId", () => {
  it("returns the first profile of the workspace's default executor", () => {
    const executors = [
      mkExecutor({ id: "exec-1", profiles: [mkExecutorProfile({ id: "profile-1" })] }),
    ];
    expect(resolveDefaultExecutorProfileId("exec-1", executors)).toBe("profile-1");
  });

  it("returns empty when the default id is not set", () => {
    expect(resolveDefaultExecutorProfileId(null, [mkExecutor()])).toBe("");
  });

  it("returns empty when the default executor no longer exists", () => {
    expect(resolveDefaultExecutorProfileId("exec-missing", [mkExecutor({ id: "exec-1" })])).toBe(
      "",
    );
  });

  it("returns empty when the default executor has no profiles", () => {
    const executors = [mkExecutor({ id: "exec-1", profiles: [] })];
    expect(resolveDefaultExecutorProfileId("exec-1", executors)).toBe("");
  });
});
