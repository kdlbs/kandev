import type { ReactNode } from "react";
import { cleanup, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  featureEnabled: true,
  listWorkspaceCanvases: vi.fn(),
  listRepositories: vi.fn(),
  listWorkflows: vi.fn(),
  listAutomations: vi.fn(),
  listSecrets: vi.fn(),
  getAzureDevOpsConfig: vi.fn(),
  fetchGitHubStatus: vi.fn(),
  getGitLabConfig: vi.fn(),
  getJiraConfig: vi.fn(),
  getLinearConfig: vi.fn(),
  listSentryInstances: vi.fn(),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.featureEnabled,
}));
vi.mock("@/lib/api/domains/canvas-api", () => ({
  listWorkspaceCanvases: mocks.listWorkspaceCanvases,
}));
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: mocks.listRepositories,
}));
vi.mock("@/lib/api/domains/kanban-api", () => ({
  listWorkflows: mocks.listWorkflows,
}));
vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomations: mocks.listAutomations,
}));
vi.mock("@/lib/api/domains/secrets-api", () => ({
  listSecrets: mocks.listSecrets,
}));
vi.mock("@/lib/api/domains/azure-devops-api", () => ({
  getAzureDevOpsConfig: mocks.getAzureDevOpsConfig,
}));
vi.mock("@/lib/api/domains/github-auth-api", () => ({
  fetchGitHubStatus: mocks.fetchGitHubStatus,
}));
vi.mock("@/lib/api/domains/gitlab-api", () => ({
  getGitLabConfig: mocks.getGitLabConfig,
}));
vi.mock("@/lib/api/domains/jira-api", () => ({
  getJiraConfig: mocks.getJiraConfig,
}));
vi.mock("@/lib/api/domains/linear-api", () => ({
  getLinearConfig: mocks.getLinearConfig,
}));
vi.mock("@/lib/api/domains/sentry-api", () => ({
  listSentryInstances: mocks.listSentryInstances,
}));
vi.mock("@/components/routing/app-link", () => ({
  default: ({
    href,
    children,
    className,
  }: {
    href: string;
    children: ReactNode;
    className?: string;
  }) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "sidebar:repositories": "Repositories",
        "workflows:workflows": "Workflows",
        "common:integrations": "Integrations",
        "common:automations": "Automations",
        "settings:secrets": "Secrets",
        "canvases:canvases": "Canvases",
      })[key] ?? key,
  }),
}));

import { useWorkspaceSectionCounts, WorkspaceSectionStats } from "./workspace-section-links";

function resetApiMocks() {
  mocks.listWorkspaceCanvases.mockReset().mockResolvedValue({ canvases: [] });
  mocks.listRepositories.mockReset().mockResolvedValue({ repositories: [] });
  mocks.listWorkflows.mockReset().mockResolvedValue({ workflows: [] });
  mocks.listAutomations.mockReset().mockResolvedValue([]);
  mocks.listSecrets.mockReset().mockResolvedValue([]);
  mocks.getAzureDevOpsConfig.mockReset().mockResolvedValue(null);
  mocks.fetchGitHubStatus.mockReset().mockResolvedValue({ authenticated: false });
  mocks.getGitLabConfig.mockReset().mockResolvedValue(null);
  mocks.getJiraConfig.mockReset().mockResolvedValue(null);
  mocks.getLinearConfig.mockReset().mockResolvedValue(null);
  mocks.listSentryInstances.mockReset().mockResolvedValue([]);
}

beforeEach(() => {
  mocks.featureEnabled = true;
  resetApiMocks();
});

afterEach(cleanup);

describe("useWorkspaceSectionCounts", () => {
  it("counts only active workspace canvases when the feature is enabled", async () => {
    mocks.listWorkspaceCanvases.mockResolvedValue({
      canvases: [
        { id: "active", status: "active", active_release_status: "valid" },
        { id: "archived", status: "archived", active_release_status: "valid" },
        { id: "pending", status: "active", active_release_status: "pending_permission" },
      ],
    });

    const { result } = renderHook(() => useWorkspaceSectionCounts("workspace-1"));

    await waitFor(() => expect(result.current.counts.canvases).toBe(1));
    expect(mocks.listWorkspaceCanvases).toHaveBeenCalledWith("workspace-1");
  });

  it("does not request canvas counts while the feature is disabled", async () => {
    mocks.featureEnabled = false;

    const { result } = renderHook(() => useWorkspaceSectionCounts("workspace-1"));

    await waitFor(() => expect(result.current.settled).toBe(true));
    expect(mocks.listWorkspaceCanvases).not.toHaveBeenCalled();
    expect(result.current.counts.canvases).toBeUndefined();
  });
});

describe("WorkspaceSectionStats", () => {
  it("shows the Canvases tile when the feature is enabled", () => {
    render(<WorkspaceSectionStats workspaceId="workspace-1" counts={{ canvases: 2 }} />);

    expect(screen.getByRole("link", { name: /Canvases/ }).getAttribute("href")).toBe(
      "/settings/workspaces/workspace-1/canvases",
    );
  });
});
