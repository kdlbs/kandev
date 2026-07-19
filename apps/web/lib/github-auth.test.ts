import { describe, expect, it } from "vitest";
import type { GitHubStatus } from "@/lib/types/github";
import {
  getGitHubMutationActor,
  getGitHubPersonalIdentityState,
  hasGitHubPersonalActor,
} from "./github-auth";

const githubHost = "github.com";

function status(overrides: Partial<GitHubStatus>): GitHubStatus {
  return {
    authenticated: true,
    username: "",
    auth_method: "github_app_installation",
    token_configured: false,
    required_scopes: [],
    ...overrides,
  };
}

describe("resolved GitHub actors", () => {
  it("does not trust active persisted connections without effective actors", () => {
    const unresolved = status({
      automation: {
        workspace_id: "ws-1",
        source: "github_app_installation",
        github_host: githubHost,
        installation_id: 42,
        status: "active",
        credential_generation: 1,
      },
      personal: {
        workspace_id: "ws-1",
        user_id: "user-1",
        github_user_id: 7,
        login: "alice",
        status: "active",
        access_expires_at: "2030-01-01T00:00:00Z",
        credential_generation: 1,
      },
    });

    expect(hasGitHubPersonalActor(unresolved)).toBe(false);
    expect(getGitHubMutationActor(unresolved)).toBeNull();
  });

  it("uses backend-resolved personal and mutation principals", () => {
    const resolved = status({
      effective_personal_actor: {
        kind: "human",
        source: "github_app_user",
        login: "alice",
        workspace_id: "ws-1",
        user_id: "user-1",
      },
      effective_manual_mutation_actor: {
        kind: "app",
        source: "github_app_installation",
        login: "acme",
        installation_id: 42,
        workspace_id: "ws-1",
      },
    });

    expect(hasGitHubPersonalActor(resolved)).toBe(true);
    expect(getGitHubMutationActor(resolved)).toBe("acme GitHub App");
  });

  it("rejects effective metadata when backend authentication failed", () => {
    const failed = status({
      authenticated: false,
      effective_personal_actor: {
        kind: "human",
        source: "github_app_user",
        login: "alice",
        workspace_id: "ws-1",
      },
      effective_manual_mutation_actor: {
        kind: "human",
        source: "github_app_user",
        login: "alice",
        workspace_id: "ws-1",
      },
    });

    expect(hasGitHubPersonalActor(failed)).toBe(false);
    expect(getGitHubMutationActor(failed)).toBeNull();
  });
});

describe("GitHub personal identity state", () => {
  it("does not trust an unresolved active personal row", () => {
    const unresolved = status({
      authenticated: false,
      automation: {
        workspace_id: "ws-1",
        source: "github_app_installation",
        github_host: githubHost,
        status: "active",
        credential_generation: 1,
      },
      personal: {
        workspace_id: "ws-1",
        user_id: "user-1",
        github_user_id: 7,
        login: "alice",
        status: "active",
        access_expires_at: "2030-01-01T00:00:00Z",
        credential_generation: 1,
      },
    });

    expect(getGitHubPersonalIdentityState(unresolved)).toEqual({
      active: false,
      actor: "Not connected",
      personalOAuthActive: false,
    });
  });

  it("represents PAT or CLI fallback as human without a Personal OAuth badge", () => {
    const fallback = status({
      auth_method: "gh_cli",
      automation: {
        workspace_id: "ws-1",
        source: "gh_cli",
        github_host: githubHost,
        login: "alice",
        status: "active",
        credential_generation: 1,
      },
      effective_personal_actor: {
        kind: "human",
        source: "gh_cli",
        login: "alice",
        workspace_id: "ws-1",
      },
    });

    expect(getGitHubPersonalIdentityState(fallback)).toEqual({
      active: true,
      actor: "alice",
      personalOAuthActive: false,
    });
  });

  it("distinguishes App-only from a verified personal App user", () => {
    const appOnly = status({
      automation: {
        workspace_id: "ws-1",
        source: "github_app_installation",
        github_host: githubHost,
        status: "active",
        credential_generation: 1,
      },
    });
    expect(getGitHubPersonalIdentityState(appOnly)).toEqual({
      active: false,
      actor: "Not connected",
      personalOAuthActive: false,
    });

    const personal = status({
      ...appOnly,
      personal: {
        workspace_id: "ws-1",
        user_id: "user-1",
        github_user_id: 7,
        login: "alice",
        status: "active",
        access_expires_at: "2030-01-01T00:00:00Z",
        credential_generation: 1,
      },
      effective_personal_actor: {
        kind: "human",
        source: "github_app_user",
        login: "alice",
        workspace_id: "ws-1",
        user_id: "user-1",
      },
    });
    expect(getGitHubPersonalIdentityState(personal)).toEqual({
      active: true,
      actor: "alice",
      personalOAuthActive: true,
    });
  });
});
