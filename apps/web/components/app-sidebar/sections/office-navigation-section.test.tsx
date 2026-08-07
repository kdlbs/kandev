import { act } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";

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

  it("renders the skills count with muted badge styling", () => {
    render(<OfficeNavigationSection collapsed={false} />);

    const badge = screen.getByText("3");
    expect(badge.classList.contains("bg-muted")).toBe(true);
    expect(badge.classList.contains("text-muted-foreground")).toBe(true);
    expect(badge.classList.contains("bg-primary")).toBe(false);
  });
});

/**
 * `workItems` / `workspaceItems` are module-scope tables. A `t()` call written
 * inside them resolves once at import and freezes at the boot locale — it still
 * renders translated, so neither lint nor the pseudo-locale can see the bug.
 * Storing the key and resolving at render is what this asserts.
 */
describe("OfficeNavigationSection locale switching", () => {
  afterEach(async () => {
    await act(async () => {
      await activateLocale("en");
    });
  });

  it("re-renders its module-scope nav labels when the locale changes", async () => {
    render(<OfficeNavigationSection collapsed={false} />);
    expect(screen.getByRole("link", { name: /Preferences/i })).toBeTruthy();

    await act(async () => {
      await activateLocale("pseudo");
    });

    expect(screen.queryByRole("link", { name: /^Preferences$/ })).toBeNull();
    expect(screen.getByRole("link", { name: /Ƥŕēƒēŕēńćēś/ })).toBeTruthy();
  });
});
