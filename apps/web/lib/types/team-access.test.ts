import { describe, expect, it } from "vitest";
import { ASSIGNABLE_WORKSPACE_ROLES, hasScope, SCOPE } from "./team-access";

describe("hasScope", () => {
  it("grants a scope the server issued", () => {
    expect(hasScope([SCOPE.workspaceRead, SCOPE.taskWrite], SCOPE.taskWrite)).toBe(true);
  });

  it("denies a scope the server withheld", () => {
    expect(hasScope([SCOPE.workspaceRead], SCOPE.sessionExec)).toBe(false);
  });

  // A payload from a backend that predates scopes, or one where the field was
  // dropped, must hide controls rather than show ones that would 403.
  it("denies every scope when the field is absent", () => {
    expect(hasScope(undefined, SCOPE.workspaceRead)).toBe(false);
    expect(hasScope([], SCOPE.workspaceManage)).toBe(false);
  });
});

describe("scope identifiers", () => {
  // These strings are compared with === against the backend registry, so a
  // rename here silently breaks every gated control rather than failing loudly.
  it("match the backend registry values", () => {
    expect(SCOPE).toEqual({
      workspaceRead: "workspace.read",
      workspaceManage: "workspace.manage",
      taskWrite: "task.write",
      sessionPrompt: "session.prompt",
      sessionControl: "session.control",
      sessionExec: "session.exec",
      repositoryManage: "repository.manage",
      secretManage: "secret.manage",
      memberManage: "member.manage",
    });
  });

  // Owner is reached by transferring ownership, never by assigning a role.
  it("does not offer owner as an assignable role", () => {
    expect(ASSIGNABLE_WORKSPACE_ROLES).toEqual(["collaborator", "viewer"]);
    expect(ASSIGNABLE_WORKSPACE_ROLES).not.toContain("owner");
  });
});
