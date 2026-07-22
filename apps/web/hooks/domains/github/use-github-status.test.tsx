import { act, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { getGitHubMutationActor } from "@/lib/github-auth";
import type { GitHubStatusResponse } from "@/lib/types/github";
import { normalizeGitHubStatus, useGitHubStatus } from "./use-github-status";

const fetchGitHubStatusMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/github-api", () => ({
  fetchGitHubStatus: fetchGitHubStatusMock,
}));

const WORKSPACE_A = "workspace-a";
const GITHUB_COM = "github.com";
const AUTOMATION_USER = "automation-user";

afterEach(() => {
  fetchGitHubStatusMock.mockReset();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

describe("useGitHubStatus workspace scoping", () => {
  it("does not clear a scoped status while the active workspace is unavailable", async () => {
    const response: GitHubStatusResponse = {
      workspace_id: WORKSPACE_A,
      authenticated: false,
      username: "",
      auth_method: "none",
      token_configured: false,
      required_scopes: [],
      automation: null,
      personal: null,
    };
    fetchGitHubStatusMock.mockResolvedValue(response);

    function ScopedProbe() {
      useGitHubStatus(WORKSPACE_A);
      return null;
    }

    function UnscopedProbe() {
      useGitHubStatus();
      return null;
    }

    render(
      <StateProvider
        initialState={{
          githubStatus: {
            byWorkspaceId: {
              [WORKSPACE_A]: {
                status: normalizeGitHubStatus(response),
                loaded: true,
                loading: false,
              },
            },
          },
        }}
      >
        <ScopedProbe />
        <UnscopedProbe />
      </StateProvider>,
    );

    await act(async () => Promise.resolve());
    expect(fetchGitHubStatusMock).not.toHaveBeenCalled();
  });

  it("keeps the newest same-workspace refresh result", async () => {
    const first = deferred<GitHubStatusResponse>();
    const second = deferred<GitHubStatusResponse>();
    fetchGitHubStatusMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <StateProvider>{children}</StateProvider>
    );
    const { result } = renderHook(() => useGitHubStatus(WORKSPACE_A), { wrapper });
    await waitFor(() => expect(fetchGitHubStatusMock).toHaveBeenCalledTimes(1));

    act(() => result.current.refresh());
    await waitFor(() => expect(fetchGitHubStatusMock).toHaveBeenCalledTimes(2));
    await act(async () =>
      second.resolve({
        workspace_id: WORKSPACE_A,
        authenticated: true,
        username: "new",
        auth_method: "pat",
        token_configured: true,
        required_scopes: [],
        effective_personal_actor: {
          kind: "human",
          source: "pat",
          login: "new",
          workspace_id: WORKSPACE_A,
        },
      }),
    );
    await waitFor(() => expect(result.current.status?.username).toBe("new"));
    await act(async () =>
      first.resolve({
        workspace_id: WORKSPACE_A,
        authenticated: true,
        username: "old",
        auth_method: "pat",
        token_configured: true,
        required_scopes: [],
        effective_personal_actor: {
          kind: "human",
          source: "pat",
          login: "old",
          workspace_id: WORKSPACE_A,
        },
      }),
    );
    expect(result.current.status?.username).toBe("new");
  });
});

describe("normalizeGitHubStatus for GitHub Apps", () => {
  it("uses the personal identity for App-backed workspaces", () => {
    const status = normalizeGitHubStatus({
      workspace_id: WORKSPACE_A,
      authenticated: true,
      username: "acme",
      auth_method: "github_app_installation",
      token_configured: false,
      required_scopes: [],
      app_available: true,
      effective_personal_actor: {
        kind: "human",
        source: "github_app_user",
        login: "alice",
        workspace_id: WORKSPACE_A,
        user_id: "user-a",
        app_registration_id: "registration-a",
      },
      automation: {
        workspace_id: WORKSPACE_A,
        source: "github_app_installation",
        github_host: GITHUB_COM,
        installation_id: 42,
        installation_account_login: "acme",
        status: "active",
        credential_generation: 1,
      },
      personal: {
        workspace_id: WORKSPACE_A,
        user_id: "user-a",
        app_registration_id: "registration-a",
        github_user_id: 7,
        login: "alice",
        status: "active",
        access_expires_at: "2030-01-01T00:00:00Z",
        credential_generation: 1,
      },
    });

    expect(status).toMatchObject({
      authenticated: true,
      auth_method: "github_app_installation",
      username: "alice",
    });
  });

  it("does not present the App installation account as a human user", () => {
    const status = normalizeGitHubStatus({
      workspace_id: WORKSPACE_A,
      authenticated: true,
      username: "acme",
      auth_method: "github_app_installation",
      token_configured: false,
      required_scopes: [],
      effective_manual_mutation_actor: {
        kind: "app",
        source: "github_app_installation",
        login: "acme",
        installation_id: 42,
        workspace_id: WORKSPACE_A,
      },
      automation: {
        workspace_id: WORKSPACE_A,
        source: "github_app_installation",
        github_host: GITHUB_COM,
        installation_id: 42,
        installation_account_login: "acme",
        status: "active",
        credential_generation: 1,
      },
      personal: null,
    });

    expect(status.authenticated).toBe(true);
    expect(status.username).toBe("");
    expect(getGitHubMutationActor(status)).toBe("acme GitHub App");
  });

  it("preserves a failed backend authentication result for active metadata", () => {
    const status = normalizeGitHubStatus({
      workspace_id: WORKSPACE_A,
      authenticated: false,
      username: "",
      auth_method: "github_app_installation",
      token_configured: false,
      required_scopes: [],
      automation: {
        workspace_id: WORKSPACE_A,
        source: "github_app_installation",
        github_host: GITHUB_COM,
        installation_id: 42,
        installation_account_login: "acme",
        status: "active",
        credential_generation: 1,
        last_error: "installation token mint failed",
      },
      personal: null,
    });

    expect(status.authenticated).toBe(false);
    expect(status.auth_method).toBe("none");
    expect(getGitHubMutationActor(status)).toBeNull();
  });
});

describe("normalizeGitHubStatus for human automation", () => {
  it("ignores an invalid personal login when resolving the effective actor", () => {
    const status = normalizeGitHubStatus({
      workspace_id: WORKSPACE_A,
      authenticated: true,
      username: AUTOMATION_USER,
      auth_method: "pat",
      token_configured: true,
      required_scopes: [],
      effective_personal_actor: {
        kind: "human",
        source: "pat",
        login: AUTOMATION_USER,
        workspace_id: WORKSPACE_A,
      },
      effective_manual_mutation_actor: {
        kind: "human",
        source: "pat",
        login: AUTOMATION_USER,
        workspace_id: WORKSPACE_A,
      },
      automation: {
        workspace_id: WORKSPACE_A,
        source: "pat",
        github_host: GITHUB_COM,
        login: AUTOMATION_USER,
        status: "active",
        credential_generation: 1,
      },
      personal: {
        workspace_id: WORKSPACE_A,
        user_id: "user-a",
        app_registration_id: "registration-a",
        github_user_id: 7,
        login: "expired-user",
        status: "invalid",
        access_expires_at: "2025-01-01T00:00:00Z",
        credential_generation: 2,
      },
    });

    expect(status.username).toBe(AUTOMATION_USER);
    expect(getGitHubMutationActor(status)).toBe(AUTOMATION_USER);
  });

  it("preserves a named CLI actor for compatibility surfaces", () => {
    const status = normalizeGitHubStatus({
      workspace_id: "workspace-b",
      authenticated: true,
      username: "bob",
      auth_method: "gh_cli",
      token_configured: false,
      required_scopes: [],
      effective_personal_actor: {
        kind: "human",
        source: "gh_cli",
        login: "bob",
        workspace_id: "workspace-b",
      },
      automation: {
        workspace_id: "workspace-b",
        source: "gh_cli",
        github_host: "github.example.com",
        login: "bob",
        status: "active",
        credential_generation: 3,
      },
      personal: null,
    });

    expect(status).toMatchObject({
      authenticated: true,
      auth_method: "gh_cli",
      username: "bob",
    });
  });
});
