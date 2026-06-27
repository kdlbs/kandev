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
      projects: false,
    } as Record<string, boolean>,
  },
  workspaces: { activeId: "workspace-1" },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
};

vi.mock("@/lib/routing/client-router", () => ({
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

import { ProjectsSection } from "./projects-section";

function renderProjectsSection() {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.office.projects("workspace-1"), { projects: [] });

  return render(
    <QueryClientProvider client={queryClient}>
      <ProjectsSection collapsed={false} />
    </QueryClientProvider>,
  );
}

describe("ProjectsSection", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("keeps the add-project action visible when the section is collapsed", () => {
    renderProjectsSection();

    const projectsHeader = screen
      .getByRole("button", { name: "Projects" })
      .closest(".group\\/section");
    expect(projectsHeader).toBeTruthy();

    within(projectsHeader as HTMLElement).getByRole("button", { name: "Add project" });
  });
});
