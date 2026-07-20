import { cleanup, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";

let pathname = "/office";

const state = {
  appSidebar: {
    sectionExpanded: {
      "office-work": true,
      "office-workspace": true,
    } as Record<string, boolean>,
  },
  workspaces: { activeId: "ws-1" as string | null },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => pathname,
}));

vi.mock("@kandev/ui/collapsible", () => ({
  Collapsible: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CollapsibleContent: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => <div className={className}>{children}</div>,
}));

import { OfficeNavigationSection } from "./office-navigation-section";

function hrefFor(label: string) {
  return screen.getByRole("link", { name: new RegExp(label, "i") }).getAttribute("href");
}

function renderOfficeNavigation() {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.office.dashboard("ws-1"), {
    task_count: 7,
    routine_count: 2,
    skill_count: 3,
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <OfficeNavigationSection collapsed={false} />
    </QueryClientProvider>,
  );
}

describe("OfficeNavigationSection", () => {
  beforeEach(() => {
    pathname = "/office";
    state.appSidebar.sectionExpanded = {
      "office-work": true,
      "office-workspace": true,
    };
  });

  afterEach(() => {
    cleanup();
  });

  it("restores links for the dedicated office pages", () => {
    renderOfficeNavigation();

    expect(hrefFor("Tasks")).toBe("/office/tasks");
    expect(hrefFor("Routines")).toBe("/office/routines");
    expect(hrefFor("Skills")).toBe("/office/workspace/skills");
    expect(hrefFor("Costs")).toBe("/office/workspace/costs");
    expect(hrefFor("Activity")).toBe("/office/workspace/activity");
    expect(hrefFor("Routing")).toBe("/office/workspace/routing");
    expect(hrefFor("Preferences")).toBe("/office/workspace/settings");
    expect(screen.queryByRole("link", { name: /Agent Topology/i })).toBeNull();
  });

  it("uses expanded defaults when old persisted sidebar state lacks new office keys", () => {
    state.appSidebar.sectionExpanded = {};

    renderOfficeNavigation();

    expect(screen.getByRole("link", { name: /Tasks/i })).toBeTruthy();
    expect(screen.getByRole("link", { name: /Preferences/i })).toBeTruthy();
  });

  it("renders the skills count with muted badge styling", () => {
    renderOfficeNavigation();

    const badge = screen.getByText("3");
    expect(badge.classList.contains("bg-muted")).toBe(true);
    expect(badge.classList.contains("text-muted-foreground")).toBe(true);
    expect(badge.classList.contains("bg-primary")).toBe(false);
  });
});
