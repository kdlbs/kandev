import { describe, expect, it } from "vitest";
import { getGitHubIntegrationStatus, getLinearHref } from "./integrations-menu";
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

describe("getLinearHref", () => {
  it("links to the Linear workspace when available", () => {
    expect(getLinearHref("workspace-1", true)).toBe("/linear");
  });

  it("links to workspace settings when Linear still needs setup", () => {
    expect(getLinearHref("workspace-1", false)).toBe("/settings/workspace/workspace-1/linear");
  });

  it("falls back to settings when there is no active workspace", () => {
    expect(getLinearHref(undefined, false)).toBe("/settings");
  });
});
