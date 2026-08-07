import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { getGitHubIntegrationStatus } from "./use-nav-availability";
import type { GitHubStatus } from "@/lib/types/github";
import type { AvailabilityKey } from "@/lib/navigation/types";

const mocks = vi.hoisted(() => {
  const workspaceId = "workspace-1";
  return {
    workspaceId,
    state: {
      workspaces: {
        activeId: workspaceId as string | null,
        items: [{ id: workspaceId }],
      },
    },
    azureDevOpsAvailable: vi.fn(() => false),
    githubStatus: vi.fn(() => ({ status: null as GitHubStatus | null, loading: false })),
    gitlabAvailable: vi.fn(() => false),
    jiraAuthed: vi.fn(() => false),
    linearAuthed: vi.fn(() => false),
    hideDisabled: vi.fn(() => false),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-availability", () => ({
  useAzureDevOpsAvailable: mocks.azureDevOpsAvailable,
}));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: mocks.githubStatus,
}));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useGitLabAvailable: mocks.gitlabAvailable,
}));
vi.mock("@/hooks/domains/jira/use-jira-availability", () => ({
  useJiraAuthed: mocks.jiraAuthed,
}));
vi.mock("@/hooks/domains/linear/use-linear-availability", () => ({
  useLinearAuthed: mocks.linearAuthed,
}));
vi.mock("@/hooks/domains/integrations/use-hide-disabled-integrations-in-nav", () => ({
  useHideDisabledIntegrationsInNav: () => ({
    hideDisabled: mocks.hideDisabled(),
    setHideDisabled: vi.fn(),
  }),
}));

// Enabled-hook mocks default to `true` everywhere: most scenarios exercise
// the `configured` axis, and defaulting `enabled` off would make every one of
// them need to opt back in just to stay green.
const enabledHookMocks = vi.hoisted(() => ({
  azureDevOps: vi.fn(() => ({ enabled: true, setEnabled: vi.fn(), loaded: true })),
  github: vi.fn(() => ({ enabled: true, setEnabled: vi.fn(), loaded: true })),
  gitlab: vi.fn(() => ({ enabled: true, setEnabled: vi.fn(), loaded: true })),
  jira: vi.fn(() => ({ enabled: true, setEnabled: vi.fn(), loaded: true })),
  linear: vi.fn(() => ({ enabled: true, setEnabled: vi.fn(), loaded: true })),
}));

vi.mock("@/hooks/domains/azure-devops/use-azure-devops-enabled", () => ({
  useAzureDevOpsEnabled: enabledHookMocks.azureDevOps,
}));
vi.mock("@/hooks/domains/github/use-github-enabled", () => ({
  useGitHubEnabled: enabledHookMocks.github,
}));
vi.mock("@/hooks/domains/gitlab/use-gitlab-enabled", () => ({
  useGitLabEnabled: enabledHookMocks.gitlab,
}));
vi.mock("@/hooks/domains/jira/use-jira-enabled", () => ({
  useJiraEnabled: enabledHookMocks.jira,
}));
vi.mock("@/hooks/domains/linear/use-linear-enabled", () => ({
  useLinearEnabled: enabledHookMocks.linear,
}));

import { useNavAvailability } from "./use-nav-availability";

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
  it("shows checking while GitHub status is loading and not configured", () => {
    expect(getGitHubIntegrationStatus(null, true)).toEqual({ ready: false, label: "Checking" });
  });

  it("treats a configured token as ready even before live auth is green", () => {
    expect(getGitHubIntegrationStatus(status({ token_configured: true }), false)).toEqual({
      ready: true,
      label: "Configured",
    });
  });

  it("uses the Connected label for authenticated status", () => {
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

const NAV_GATED_KEYS: AvailabilityKey[] = ["azure-devops", "github", "gitlab", "jira", "linear"];

const ENABLED_MOCK_BY_KEY = {
  "azure-devops": enabledHookMocks.azureDevOps,
  github: enabledHookMocks.github,
  gitlab: enabledHookMocks.gitlab,
  jira: enabledHookMocks.jira,
  linear: enabledHookMocks.linear,
};

function setConfigured(key: AvailabilityKey, configured: boolean) {
  switch (key) {
    case "azure-devops":
      mocks.azureDevOpsAvailable.mockReturnValue(configured);
      return;
    case "github":
      mocks.githubStatus.mockReturnValue({
        status: configured ? status({ authenticated: true }) : status({}),
        loading: false,
      });
      return;
    case "gitlab":
      mocks.gitlabAvailable.mockReturnValue(configured);
      return;
    case "jira":
      mocks.jiraAuthed.mockReturnValue(configured);
      return;
    case "linear":
      mocks.linearAuthed.mockReturnValue(configured);
      return;
  }
}

function setEnabled(key: AvailabilityKey, enabled: boolean) {
  ENABLED_MOCK_BY_KEY[key].mockReturnValue({ enabled, setEnabled: vi.fn(), loaded: true });
}

describe("useNavAvailability", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.state.workspaces.activeId = mocks.workspaceId;
    mocks.state.workspaces.items = [{ id: mocks.workspaceId }];
    mocks.githubStatus.mockReturnValue({ status: null, loading: false });
    mocks.hideDisabled.mockReturnValue(false);
    for (const key of NAV_GATED_KEYS) setEnabled(key, true);
  });

  it("scopes workspace integrations to the active workspace", () => {
    renderHook(() => useNavAvailability());

    expect(mocks.jiraAuthed).toHaveBeenCalledWith(mocks.workspaceId);
    expect(mocks.linearAuthed).toHaveBeenCalledWith(mocks.workspaceId);
    expect(mocks.azureDevOpsAvailable).toHaveBeenCalledWith(mocks.workspaceId);
  });

  it("falls back to default workspace resolution for a stale active id", () => {
    mocks.state.workspaces.items = [{ id: "workspace-2" }];

    renderHook(() => useNavAvailability());

    expect(mocks.jiraAuthed).toHaveBeenCalledWith(null);
    expect(mocks.linearAuthed).toHaveBeenCalledWith(null);
    expect(mocks.azureDevOpsAvailable).toHaveBeenCalledWith(null);
  });

  describe.each(NAV_GATED_KEYS)("decoupling enabled from nav visibility for %s", (key) => {
    it("hides an unconfigured integration regardless of enabled/hideDisabled", () => {
      setConfigured(key, false);
      setEnabled(key, true);
      mocks.hideDisabled.mockReturnValue(false);
      const { result: hideDisabledOff } = renderHook(() => useNavAvailability());
      expect(hideDisabledOff.current[key]).toBe(false);

      mocks.hideDisabled.mockReturnValue(true);
      const { result: hideDisabledOn } = renderHook(() => useNavAvailability());
      expect(hideDisabledOn.current[key]).toBe(false);
    });

    it('with "hide disabled" off (default), a configured-but-disabled integration stays visible', () => {
      setConfigured(key, true);
      setEnabled(key, false);
      mocks.hideDisabled.mockReturnValue(false);

      const { result } = renderHook(() => useNavAvailability());

      expect(result.current[key]).toBe(true);
    });

    it('with "hide disabled" on, a configured-but-disabled integration is hidden', () => {
      setConfigured(key, true);
      setEnabled(key, false);
      mocks.hideDisabled.mockReturnValue(true);

      const { result } = renderHook(() => useNavAvailability());

      expect(result.current[key]).toBe(false);
    });

    it('with "hide disabled" on, a configured and enabled integration stays visible', () => {
      setConfigured(key, true);
      setEnabled(key, true);
      mocks.hideDisabled.mockReturnValue(true);

      const { result } = renderHook(() => useNavAvailability());

      expect(result.current[key]).toBe(true);
    });
  });
});
