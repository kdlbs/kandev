import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

let pathname = "/office";

const state = {
  appSidebar: {
    sectionExpanded: {
      "office-work": true,
      "office-workspace": true,
    } as Record<string, boolean>,
  },
  office: {
    dashboard: {
      task_count: 7,
      routine_count: 2,
      skill_count: 3,
    },
  },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof state) => unknown) => selector(state),
}));

vi.mock("next/navigation", () => ({
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
    render(<OfficeNavigationSection collapsed={false} />);

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

    render(<OfficeNavigationSection collapsed={false} />);

    expect(screen.getByRole("link", { name: /Tasks/i })).toBeTruthy();
    expect(screen.getByRole("link", { name: /Preferences/i })).toBeTruthy();
  });
});
