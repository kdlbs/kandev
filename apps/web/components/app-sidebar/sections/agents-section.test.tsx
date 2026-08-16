import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import { agentProfileId, workspaceId } from "@/lib/types/ids";
import { listAgentProfiles } from "@/lib/api/domains/office-api";

const routerMock = vi.hoisted(() => ({
  push: vi.fn(),
}));

const defaultWorkspaceId = workspaceId("workspace-1");
const staleWorkspaceId = workspaceId("old-workspace");
const defaultAgentId = "claude";
const defaultAgentDisplayName = "Claude";
const defaultAgentModel = "claude-sonnet-4-5";
const timestamp = "2026-01-01T00:00:00Z";

const state = {
  appSidebar: {
    sectionExpanded: {
      agents: true,
    } as Record<string, boolean>,
  },
  office: {
    agentProfilesByWorkspaceId: {} as Record<string, AgentProfile[]>,
    projectsByWorkspaceId: {} as Record<string, Array<unknown>>,
    inboxItemsByWorkspaceId: {} as Record<string, Array<unknown>>,
    inboxCountByWorkspaceId: {} as Record<string, number>,
  },
  workspaces: {
    activeId: defaultWorkspaceId as string | null,
  },
  setOfficeAgentProfiles: vi.fn(),
  setProjects: vi.fn(),
  setInboxItems: vi.fn(),
  setInboxCount: vi.fn(),
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
  sessions: {
    byId: {},
  },
  taskSessions: {
    items: {},
  },
};

const noAgentsText = "No agents yet";
const createAgentProfile = ({
  id,
  workspace,
  name,
}: {
  id: string;
  workspace: string;
  name: string;
}): AgentProfile =>
  ({
    id: agentProfileId(id),
    workspaceId: workspaceId(workspace),
    name,
    role: "worker",
    status: "idle",
    budgetMonthlyCents: 0,
    maxConcurrentSessions: 1,
    agentId: defaultAgentId,
    agentDisplayName: defaultAgentDisplayName,
    model: defaultAgentModel,
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: timestamp,
    updatedAt: timestamp,
  }) as AgentProfile;

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/office",
  useRouter: () => routerMock,
}));

vi.mock("@/hooks/use-in-office", () => ({
  useInOffice: () => true,
}));

vi.mock("@/hooks/use-office-refetch", () => ({
  useOfficeRefetch: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-api", () => ({
  listAgentProfiles: vi.fn(() => Promise.resolve({ agents: [] })),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
  useAppStoreApi: () => ({
    getState: () => ({
      workspaces: {
        activeId: state.workspaces.activeId,
      },
    }),
  }),
}));

vi.mock("@kandev/ui/collapsible", () => ({
  Collapsible: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CollapsibleContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { AgentsSection } from "./agents-section";

const resetOfficeState = () => {
  state.office.agentProfilesByWorkspaceId = {};
  state.office.projectsByWorkspaceId = {};
  state.office.inboxItemsByWorkspaceId = {};
  state.office.inboxCountByWorkspaceId = {};
  state.workspaces.activeId = defaultWorkspaceId;
};

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  resetOfficeState();
});

describe("AgentsSection header", () => {
  it("renders Agent Topology as the header action before Add agent", () => {
    render(<AgentsSection collapsed={false} />);

    const agentsHeader = screen.getByRole("button", { name: "Agents" }).closest(".group\\/section");
    expect(agentsHeader).toBeTruthy();

    const topology = within(agentsHeader as HTMLElement).getByRole("link", {
      name: "Agent topology",
    });
    const addAgent = within(agentsHeader as HTMLElement).getByRole("button", { name: "Add agent" });

    expect(topology.getAttribute("href")).toBe("/office/workspace/org");
    expect(topology.compareDocumentPosition(addAgent) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  });
});

describe("AgentsSection workspace scoping", () => {
  it("does not render stale agent links when no office workspace is active", () => {
    state.workspaces.activeId = null;
    state.office.agentProfilesByWorkspaceId = {
      [staleWorkspaceId]: [
        createAgentProfile({
          id: "stale-agent",
          workspace: staleWorkspaceId,
          name: "Stale Agent",
        }),
      ],
    };

    render(<AgentsSection collapsed={false} />);

    expect(screen.queryByRole("link", { name: /stale agent/i })).toBeNull();
    expect(screen.getByText(noAgentsText)).toBeTruthy();
  });

  it("renders only the active workspace's agents, without clearing the other's", () => {
    // Before the office collections were keyed by workspace, this section had
    // to actively wipe the store when the active workspace went away. Keying
    // makes the wipe unnecessary: the other workspace's agents simply are not
    // reachable through the selector, and their data survives a switch back.
    state.workspaces.activeId = defaultWorkspaceId;
    state.office.agentProfilesByWorkspaceId = {
      [defaultWorkspaceId]: [
        createAgentProfile({
          id: "active-agent",
          workspace: defaultWorkspaceId,
          name: "Active Agent",
        }),
      ],
      [staleWorkspaceId]: [
        createAgentProfile({
          id: "other-agent",
          workspace: staleWorkspaceId,
          name: "Other Agent",
        }),
      ],
    };

    const { rerender } = render(<AgentsSection collapsed={false} />);
    expect(screen.getByRole("link", { name: /active agent/i })).toBeTruthy();
    expect(screen.queryByRole("link", { name: /other agent/i })).toBeNull();

    state.workspaces.activeId = staleWorkspaceId;
    rerender(<AgentsSection collapsed={false} />);

    expect(screen.getByRole("link", { name: /other agent/i })).toBeTruthy();
    expect(screen.queryByRole("link", { name: /active agent/i })).toBeNull();
    expect(state.setOfficeAgentProfiles).not.toHaveBeenCalledWith(expect.anything(), []);
  });

  it("fetches nothing of its own", () => {
    // Agent loading moved to `useOfficeWorkspaceData`, so this section renders
    // whatever the store holds and never issues a request. The race and
    // request-sequencing cases that used to live here moved with it, to
    // `hooks/use-office-workspace-data.test.ts`.
    render(<AgentsSection collapsed={false} />);

    expect(vi.mocked(listAgentProfiles)).not.toHaveBeenCalled();
    expect(state.setOfficeAgentProfiles).not.toHaveBeenCalled();
  });
});

describe("AgentsSection error badge", () => {
  const renderWithFailedRuns = (failedRuns: number) => {
    state.office.agentProfilesByWorkspaceId = {
      [defaultWorkspaceId]: [
        createAgentProfile({ id: "agent-1", workspace: defaultWorkspaceId, name: "Alfa" }),
      ],
    };
    state.office.inboxItemsByWorkspaceId = {
      [defaultWorkspaceId]: Array.from({ length: failedRuns }, () => ({
        type: "agent_run_failed",
        payload: { agent_profile_id: agentProfileId("agent-1") },
      })),
    };
    render(<AgentsSection collapsed={false} />);
  };

  // The badge used to concatenate an English "s" at the call site, which is
  // untranslatable — it must go through a `count` plural instead.
  it("uses the singular form for one failed run", () => {
    renderWithFailedRuns(1);

    expect(screen.getByText("1 error")).toBeTruthy();
  });

  it("uses the plural form for several failed runs", () => {
    renderWithFailedRuns(3);

    expect(screen.getByText("3 errors")).toBeTruthy();
  });
});
