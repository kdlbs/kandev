import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ isMobile: false, isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.isFinePointer,
  }),
}));

import { CollapsibleFileHeader } from "./collapsible-file-header";

afterEach(() => {
  cleanup();
  mocks.isMobile = false;
  mocks.isFinePointer = true;
});

const baseProps = {
  filePath: ".agents/skills/review/SKILL.md",
  repositoryName: "frontend",
  status: "modified" as const,
  collapsed: false,
  expandLabel: "Expand .agents/skills/review/SKILL.md",
  collapseLabel: "Collapse .agents/skills/review/SKILL.md",
  onToggleCollapse: vi.fn(),
  desktopStats: <span data-testid="desktop-stats">+2 / -1</span>,
  mobileStats: <span data-testid="mobile-stats">+2 / -1</span>,
  desktopMetadata: <span data-testid="desktop-metadata">stale</span>,
  mobileMetadata: <span data-testid="mobile-metadata">stale</span>,
  actions: <button type="button">Tools</button>,
  actionsTestId: "file-actions",
};

describe("CollapsibleFileHeader", () => {
  it("forwards custom classes to the desktop identity container", () => {
    render(<CollapsibleFileHeader {...baseProps} className="desktop-custom" />);

    expect(screen.getByTestId("collapsible-file-identity").className).toContain("desktop-custom");
  });

  it("keeps the identity toggle and action slots separate on desktop", () => {
    render(<CollapsibleFileHeader {...baseProps} />);

    const identity = screen.getByTestId("collapsible-file-identity");
    expect(identity.contains(screen.getByRole("button", { name: baseProps.collapseLabel }))).toBe(
      true,
    );
    expect(screen.getByTestId("file-actions").contains(identity)).toBe(false);
    expect(screen.getByTestId("desktop-stats")).toBeTruthy();
    expect(screen.getByTestId("desktop-metadata")).toBeTruthy();
    expect(screen.getByText("SKILL.md")).toBeTruthy();
    expect(screen.getByText(".agents/skills/review")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: baseProps.collapseLabel }));
    expect(baseProps.onToggleCollapse).toHaveBeenCalledOnce();
  });

  it("uses a touch-sized mobile identity and embeds actions in it", () => {
    mocks.isMobile = true;
    render(<CollapsibleFileHeader {...baseProps} />);

    const identity = screen.getByTestId("collapsible-file-identity");
    expect(identity.contains(screen.getByRole("button", { name: baseProps.collapseLabel }))).toBe(
      true,
    );
    expect(identity.contains(screen.getByRole("button", { name: "Tools" }))).toBe(true);
    expect(screen.queryByTestId("file-actions")).toBeNull();
    expect(screen.getByTestId("mobile-stats")).toBeTruthy();
    expect(screen.getByTestId("mobile-metadata")).toBeTruthy();
    expect(screen.getByRole("button", { name: baseProps.collapseLabel }).className).toContain(
      "min-h-11",
    );
  });

  it("uses a touch-sized desktop identity for coarse-pointer tablets", () => {
    mocks.isFinePointer = false;
    render(<CollapsibleFileHeader {...baseProps} />);

    expect(screen.getByRole("button", { name: baseProps.collapseLabel }).className).toContain(
      "min-h-11",
    );
  });
});
