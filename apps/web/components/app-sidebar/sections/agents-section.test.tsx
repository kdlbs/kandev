import { cleanup, render, screen, within } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";

const routerMock = vi.hoisted(() => ({
  push: vi.fn(),
}));

const state = {
  appSidebar: {
    sectionExpanded: {
      agents: true,
    } as Record<string, boolean>,
  },
  workspaces: {
    activeId: "workspace-1",
  },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
  sessions: {
    byId: {},
  },
};

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/office",
  useRouter: () => routerMock,
}));

vi.mock("@/hooks/use-in-office", () => ({
  useInOffice: () => true,
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

function renderAgentsSection() {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.office.agents("workspace-1"), { agents: [] });
  queryClient.setQueryData(qk.office.inbox("workspace-1"), { items: [], total_count: 0 });

  return render(
    <QueryClientProvider client={queryClient}>
      <AgentsSection collapsed={false} />
    </QueryClientProvider>,
  );
}

describe("AgentsSection", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders Agent Topology as the header action before Add agent", () => {
    renderAgentsSection();

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
