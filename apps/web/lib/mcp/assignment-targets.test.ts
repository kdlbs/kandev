import { expect, it } from "vitest";
import type { MCPProfileOwner } from "@/lib/types/http-mcp";
import { agentProfileId, repositoryId, sessionId, taskId, workspaceId } from "@/lib/types/ids";
import {
  profileAssignmentTargets,
  repositoryAssignmentTargets,
  sessionAssignmentTargets,
} from "./assignment-targets";

const profile = (id: string, fields: Partial<MCPProfileOwner> = {}): MCPProfileOwner => ({
  id,
  agent_id: "agent",
  agent_name: "Agent",
  name: id,
  enabled: true,
  kind: "concrete",
  supports_mcp: true,
  workspace_id: "",
  ...fields,
});
const owners = [
  profile("global"),
  profile("local", { workspace_id: "ws" }),
  profile("foreign", { workspace_id: "other" }),
  profile("disabled", { enabled: false }),
  profile("dynamic", { kind: "dynamic" }),
];

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.2, AC-AGENTS-MCP-ASSIGNMENT-003.3
it("offers enabled global and matching workspace profiles with explicit context", () => {
  expect(profileAssignmentTargets("ws", owners)).toEqual([
    {
      scope: "profile",
      workspaceId: "ws",
      ownerId: "global",
      agentId: "agent",
      label: "global",
      detail: "Agent",
    },
    {
      scope: "profile",
      workspaceId: "ws",
      ownerId: "local",
      agentId: "agent",
      label: "local",
      detail: "Agent",
    },
  ]);
  expect(profileAssignmentTargets("ws", [{ ...owners[0], supports_mcp: false }])).toEqual([]);
});

it("rejects missing profile-to-agent identity and deduplicates exact owners", () => {
  const valid = profile("valid");
  expect(
    profileAssignmentTargets("ws", [valid, valid, profile("missing", { agent_id: "" })]).map(
      (item) => item.ownerId,
    ),
  ).toEqual(["valid"]);
});

it("keeps repositories inside their authorized workspace without inventing an enabled flag", () => {
  expect(
    repositoryAssignmentTargets("ws", [
      { id: repositoryId("repo"), name: "Docs", workspace_id: workspaceId("ws") },
      { id: repositoryId("other"), name: "Other", workspace_id: workspaceId("other") },
    ]),
  ).toEqual([{ scope: "repository", workspaceId: "ws", ownerId: "repo", label: "Docs" }]);
});

const task = {
  id: taskId("task"),
  workspace_id: workspaceId("ws"),
  title: "Work",
  archived_at: null,
};
const session = {
  id: sessionId("session"),
  task_id: task.id,
  agent_profile_id: agentProfileId("global"),
  name: "Conversation",
  state: "RUNNING" as const,
};

it("offers the exact running session and excludes terminal, passthrough, and disabled-profile sessions", () => {
  const profiles = profileAssignmentTargets("ws", owners);
  const sessions = [
    session,
    ...(["COMPLETED", "FAILED", "CANCELLED"] as const).map((state) => ({
      ...session,
      id: sessionId(state),
      state,
    })),
    { ...session, id: sessionId("passthrough"), is_passthrough: true },
    { ...session, id: sessionId("legacy-pty"), agent_profile_snapshot: { cli_passthrough: true } },
    { ...session, id: sessionId("disabled"), agent_profile_id: agentProfileId("disabled") },
    { ...session, id: sessionId("foreign-task"), task_id: taskId("other") },
  ];
  expect(sessionAssignmentTargets("ws", task, sessions, profiles)).toEqual([
    {
      scope: "task_session",
      workspaceId: "ws",
      ownerId: "session",
      taskId: "task",
      label: "Conversation",
      detail: "global · session",
    },
  ]);
});

it("does not offer any session from an unavailable task or missing current profile", () => {
  const profiles = profileAssignmentTargets("ws", owners);
  for (const owner of [
    { ...task, workspace_id: workspaceId("other") },
    { ...task, archived_at: "2026-10-25T00:00:00Z" },
    { ...task, workspace_orphaned: true },
  ])
    expect(sessionAssignmentTargets("ws", owner, [session], profiles)).toEqual([]);
  expect(sessionAssignmentTargets("ws", task, [session], [])).toEqual([]);
});

it("distinguishes unnamed conversations that share a profile in choices and review details", () => {
  const profiles = profileAssignmentTargets("ws", owners);
  const targets = sessionAssignmentTargets(
    "ws",
    task,
    [
      { ...session, id: sessionId("same-prefix-first"), name: "" },
      { ...session, id: sessionId("same-prefix-second"), name: "" },
    ],
    profiles,
  );
  expect(targets.map((target) => target.detail)).toEqual([
    "global · same-prefix-first",
    "global · same-prefix-second",
  ]);
});
