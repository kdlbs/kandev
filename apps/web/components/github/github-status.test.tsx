import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type { GitHubStatus } from "@/lib/types/github";
import { GitHubPersonalSettings } from "./github-status";

const mocks = vi.hoisted(() => ({ status: null as GitHubStatus | null }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: mocks.status, loaded: true, loading: false, refresh: vi.fn() }),
}));

afterEach(() => cleanup());

function renderPersonal() {
  return render(
    <ToastProvider>
      <GitHubPersonalSettings workspaceId="workspace-1" />
    </ToastProvider>,
  );
}

function status(source: "pat" | "gh_cli" | "github_app_installation"): GitHubStatus {
  return {
    workspace_id: "workspace-1",
    automation: {
      workspace_id: "workspace-1",
      source,
      github_host: "github.com",
      login: "octocat",
      status: "active",
      credential_generation: 1,
    },
    personal: null,
    app_available: source === "github_app_installation",
    effective_personal_actor: {
      kind: "human",
      source,
      login: "octocat",
      workspace_id: "workspace-1",
    },
    authenticated: true,
    username: "octocat",
    auth_method: source,
    token_configured: source === "pat",
    required_scopes: [],
  };
}

describe("GitHubPersonalSettings", () => {
  it.each(["pat", "gh_cli"] as const)("explains the shared human identity for %s", (source) => {
    mocks.status = status(source);
    renderPersonal();
    expect(screen.getByText("My GitHub identity")).toBeTruthy();
    expect(screen.getByText(/same human account selected for workspace access/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Connect identity/ })).toBeNull();
  });

  it("offers a separate identity only for App automation", () => {
    mocks.status = status("github_app_installation");
    renderPersonal();
    expect(screen.getByRole("button", { name: /Connect identity/ })).toBeTruthy();
    expect(screen.getByText(/never given to managed agents/)).toBeTruthy();
  });
});
