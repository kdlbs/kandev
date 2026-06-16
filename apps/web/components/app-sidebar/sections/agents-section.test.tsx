import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const routerMock = vi.hoisted(() => ({
  push: vi.fn(),
}));

const state = {
  appSidebar: {
    sectionExpanded: {
      agents: true,
    } as Record<string, boolean>,
  },
  office: {
    agentProfiles: [],
    inboxItems: [],
  },
  workspaces: {
    activeId: "workspace-1",
  },
  setOfficeAgentProfiles: vi.fn(),
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
  sessions: {
    byId: {},
  },
};

vi.mock("next/navigation", () => ({
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

describe("AgentsSection", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders Agent Topology as the header action before Add agent", () => {
    render(<AgentsSection collapsed={false} />);

    const agentsHeader = screen.getByRole("button", { name: "Agents" }).closest(".group\\/section");
    expect(agentsHeader).toBeTruthy();

    const topology = within(agentsHeader as HTMLElement).getByRole("link", {
      name: "Agent topology",
    });
    const addAgent = within(agentsHeader as HTMLElement).getByRole("button", {
      name: "Add agent",
    });

    expect(topology.getAttribute("href")).toBe("/office/workspace/org");
    expect(topology.compareDocumentPosition(addAgent) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
  });
});
