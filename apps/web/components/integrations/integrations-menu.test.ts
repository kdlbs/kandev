import { describe, expect, it } from "vitest";
import { getAvailableIntegrationLinks, getGitHubIntegrationStatus } from "./integrations-menu";
import type { GitHubStatus } from "@/lib/types/github";

function status(overrides: Partial<GitHubStatus>): GitHubStatus {
  return {
    authenticated: false,
    username: "",
    auth_method: "none",
    token_configured: false,
    required_scopes: [],
    ...overrides,
  };
}

describe("getGitHubIntegrationStatus", () => {
  it("treats a configured token as ready even before live auth is green", () => {
    expect(getGitHubIntegrationStatus(status({ token_configured: true }), false)).toEqual({
      ready: true,
      label: "Configured",
    });
  });

  it("uses the GitHub page for authenticated status", () => {
    expect(getGitHubIntegrationStatus(status({ authenticated: true }), false)).toEqual({
      ready: true,
      label: "Connected",
    });
  });

  it("shows setup only when no auth or token is configured", () => {
    expect(getGitHubIntegrationStatus(status({}), false)).toEqual({
      ready: false,
      label: "Setup",
    });
  });
});

describe("getAvailableIntegrationLinks", () => {
  it("returns only configured integration destinations", () => {
    expect(
      getAvailableIntegrationLinks({
        githubReady: true,
        jiraAvailable: false,
        linearAvailable: true,
      }),
    ).toEqual([
      { id: "github", label: "GitHub", href: "/github" },
      { id: "linear", label: "Linear", href: "/linear" },
    ]);
  });

  it("returns no setup destinations when integrations are unavailable", () => {
    expect(
      getAvailableIntegrationLinks({
        githubReady: false,
        jiraAvailable: false,
        linearAvailable: false,
      }),
    ).toEqual([]);
  });
});
