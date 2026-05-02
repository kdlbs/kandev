import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getAvailableIntegrationLinks,
  getGitHubIntegrationStatus,
  IntegrationsMenu,
} from "./integrations-menu";
import type { GitHubStatus } from "@/lib/types/github";

const useGitHubStatusMock = vi.hoisted(() => vi.fn());
const useJiraAvailableMock = vi.hoisted(() => vi.fn());
const useLinearAvailableMock = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: useGitHubStatusMock,
}));

vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAvailable: useJiraAvailableMock,
}));

vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAvailable: useLinearAvailableMock,
}));

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

function mockAvailability({
  githubReady,
  jiraAvailable,
  linearAvailable,
}: {
  githubReady: boolean;
  jiraAvailable: boolean;
  linearAvailable: boolean;
}) {
  useGitHubStatusMock.mockReturnValue({
    status: githubReady ? status({ token_configured: true }) : status({}),
    loading: false,
  });
  useJiraAvailableMock.mockReturnValue(jiraAvailable);
  useLinearAvailableMock.mockReturnValue(linearAvailable);
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("getGitHubIntegrationStatus", () => {
  it("shows checking while GitHub status is loading and not configured", () => {
    expect(getGitHubIntegrationStatus(status({}), true)).toEqual({
      ready: false,
      label: "Checking",
    });
  });

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

describe("IntegrationsMenu", () => {
  it("opens configured integration links on hover", async () => {
    mockAvailability({ githubReady: true, jiraAvailable: true, linearAvailable: false });

    render(createElement(IntegrationsMenu, {}));

    const trigger = screen.getByRole("button", { name: "Integrations" });
    expect(screen.queryByText("GitHub")).toBeNull();

    fireEvent.pointerEnter(trigger);

    expect(await screen.findByText("GitHub")).toBeTruthy();
    expect(screen.getByText("Jira")).toBeTruthy();
    expect(screen.queryByText("Linear")).toBeNull();
  });

  it("does not render when no integrations are configured", () => {
    mockAvailability({ githubReady: false, jiraAvailable: false, linearAvailable: false });

    render(createElement(IntegrationsMenu, {}));

    expect(screen.queryByRole("button", { name: "Integrations" })).toBeNull();
  });
});
