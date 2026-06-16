import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const routerMock = vi.hoisted(() => ({
  push: vi.fn(),
}));

const state = {
  appSidebar: {
    sectionExpanded: {
      projects: false,
    } as Record<string, boolean>,
  },
  office: {
    projects: [],
  },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
};

vi.mock("next/navigation", () => ({
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

describe("ProjectsSection", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("keeps the add-project action visible when the section is collapsed", () => {
    render(<ProjectsSection collapsed={false} />);

    const projectsHeader = screen
      .getByRole("button", { name: "Projects" })
      .closest(".group\\/section");
    expect(projectsHeader).toBeTruthy();

    within(projectsHeader as HTMLElement).getByRole("button", { name: "Add project" });
  });
});
