import { describe, expect, it } from "vitest";
import { getGitCredentialDisplay } from "./changes-git-credential-display";

describe("getGitCredentialDisplay", () => {
  it("renders the managed workspace method without exposing credentials", () => {
    expect(
      getGitCredentialDisplay({
        git_credential_snapshot: {
          source: "workspace",
          workspace_method: "gh_cli",
          transport: "managed_https",
        },
      }),
    ).toEqual({
      source: "Managed workspace identity",
      detail: "workspace GitHub CLI account",
      transport: "Kandev-managed GitHub HTTPS and gh",
    });
  });

  it("renders executor profile precedence", () => {
    expect(
      getGitCredentialDisplay({
        git_credential_snapshot: {
          source: "executor_profile",
          actor: "runtime_selected",
          transport: "profile_token",
        },
      }),
    ).toEqual({
      source: "Executor-profile token",
      detail: "runtime_selected",
      transport: "executor-profile token",
    });
  });

  it("does not display absent or malformed snapshots", () => {
    expect(getGitCredentialDisplay(null)).toBeNull();
    expect(getGitCredentialDisplay({ git_credential_snapshot: "secret" })).toBeNull();
  });
});
