import { act } from "react";
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
type Deferred<T> = {
  resolve: (value: T) => void;
  promise: Promise<T>;
};
const createDeferred = <T,>() => {
  let resolve: (value: T) => void = () => {};
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve } as Deferred<T>;
};

const state = {
  appSidebar: {
    sectionExpanded: {
      agents: true,
    } as Record<string, boolean>,
  },
  office: {
    agentProfiles: [] as AgentProfile[],
    projects: [] as Array<unknown>,
    inboxItems: [] as Array<unknown>,
    inboxCount: 0,
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
  state.office.agentProfiles = [];
  state.office.projects = [];
  state.office.inboxItems = [];
  state.office.inboxCount = 0;
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

describe("AgentsSection stale state cleanup", () => {
  it("does not render stale agent links when no office workspace is active", () => {
    state.workspaces.activeId = null;
    state.office.agentProfiles = [
      createAgentProfile({
        id: "stale-agent",
        workspace: staleWorkspaceId,
        name: "Stale Agent",
      }),
    ];

    render(<AgentsSection collapsed={false} />);

    expect(screen.queryByRole("link", { name: /stale agent/i })).toBeNull();
    expect(screen.getByText(noAgentsText)).toBeTruthy();
  });

  it("clears all office data when workspace becomes inactive", () => {
    state.workspaces.activeId = defaultWorkspaceId;
    state.office.agentProfiles = [
      createAgentProfile({
        id: "active-agent",
        workspace: defaultWorkspaceId,
        name: "Active Agent",
      }),
    ];
    state.office.projects = [{ id: "project-1" }];
    state.office.inboxItems = [{ id: "item-1" }];
    state.office.inboxCount = 2;

    const { rerender } = render(<AgentsSection collapsed={false} />);
    state.workspaces.activeId = null;
    rerender(<AgentsSection collapsed={false} />);

    expect(state.setOfficeAgentProfiles).toHaveBeenCalledWith([]);
    expect(state.setProjects).toHaveBeenCalledWith([]);
    expect(state.setInboxItems).toHaveBeenCalledWith([]);
    expect(state.setInboxCount).toHaveBeenCalledWith(0);
  });

  it("does not overwrite stale agent state when workspace changes mid-fetch", async () => {
    let resolveAgents: (value: { agents: AgentProfile[] }) => void = () => {};
    const pending = new Promise<{ agents: AgentProfile[] }>((resolve) => {
      resolveAgents = resolve;
    });

    vi.mocked(listAgentProfiles).mockReturnValue(pending);

    state.workspaces.activeId = staleWorkspaceId;
    render(<AgentsSection collapsed={false} />);

    state.workspaces.activeId = null;
    const staleAgents = {
      agents: [
        createAgentProfile({
          id: "race-agent",
          workspace: staleWorkspaceId,
          name: "Race Agent",
        }),
      ],
    };

    await act(async () => {
      resolveAgents(staleAgents);
      await pending;
    });

    expect(state.setOfficeAgentProfiles).not.toHaveBeenCalled();
  });
});

describe("AgentsSection request sequencing", () => {
  it("applies only the latest response after rapid workspace switches", async () => {
    const request1 = createDeferred<{ agents: AgentProfile[] }>();
    const request2 = createDeferred<{ agents: AgentProfile[] }>();
    const request3 = createDeferred<{ agents: AgentProfile[] }>();
    const agentFromFirstRequest = createAgentProfile({
      id: "agent-a",
      workspace: defaultWorkspaceId,
      name: "First Agent",
    });
    const agentFromSecondRequest = createAgentProfile({
      id: "agent-b",
      workspace: staleWorkspaceId,
      name: "Second Agent",
    });
    const agentFromThirdRequest = createAgentProfile({
      id: "agent-c",
      workspace: defaultWorkspaceId,
      name: "Third Agent",
    });

    vi.mocked(listAgentProfiles)
      .mockReturnValueOnce(request1.promise)
      .mockReturnValueOnce(request2.promise)
      .mockReturnValueOnce(request3.promise);

    state.workspaces.activeId = defaultWorkspaceId;
    const { rerender } = render(<AgentsSection collapsed={false} />);
    state.workspaces.activeId = staleWorkspaceId;
    rerender(<AgentsSection collapsed={false} />);
    state.workspaces.activeId = defaultWorkspaceId;
    rerender(<AgentsSection collapsed={false} />);

    await act(async () => {
      request1.resolve({ agents: [agentFromFirstRequest] });
      request2.resolve({ agents: [agentFromSecondRequest] });
      request3.resolve({ agents: [agentFromThirdRequest] });
      await request1.promise;
      await request2.promise;
      await request3.promise;
    });

    expect(state.setOfficeAgentProfiles).toHaveBeenCalledTimes(1);
    expect(state.setOfficeAgentProfiles).toHaveBeenCalledWith([agentFromThirdRequest]);
  });
});
