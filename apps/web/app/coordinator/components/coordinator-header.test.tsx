import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { CoordinatorHeader } from "./coordinator-header";

let isFinePointer = true;

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    breakpoint: "desktop",
    isMobile: false,
    isTablet: false,
    isDesktop: true,
    isCompactDesktop: false,
    isFullDesktop: true,
    isFinePointer,
    usesDesktopWorkbench: true,
  }),
}));

afterEach(() => {
  cleanup();
  isFinePointer = true;
});

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "co-1",
    workspace_id: "ws-1",
    name: "Planner",
    agent_profile_id: "a-1",
    executor_profile_id: "e-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

describe("CoordinatorHeader", () => {
  it("shows the coordinator's name as plain text with a single coordinator", () => {
    render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage={false}
      />,
    );
    expect(screen.getByText("Planner")).not.toBeNull();
    expect(screen.queryByTestId("coordinator-selector")).toBeNull();
  });

  it("shows a selector with several coordinators", () => {
    const other = coordinator({ id: "co-2", name: "Reviewer" });
    render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator(), other]}
        workspaceId="ws-1"
        view="needs-you"
        canManage={false}
      />,
    );
    expect(screen.getByTestId("coordinator-selector")).not.toBeNull();
  });

  it("shows Configure only when canManage is true", () => {
    const { rerender } = render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage={false}
      />,
    );
    expect(screen.queryByText("Configure")).toBeNull();

    rerender(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage
      />,
    );
    expect(screen.getByRole("link", { name: "Configure" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/coordinators/co-1",
    );
  });

  it("sizes Configure for touch on a coarse pointer", () => {
    isFinePointer = false;
    render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage
      />,
    );
    const configureLink = screen.getByRole("link", { name: "Configure" });
    expect(configureLink.className).toContain("min-h-11");
    expect(configureLink.className).toContain("min-w-11");
  });

  it("does not force touch sizing for Configure on a fine pointer", () => {
    isFinePointer = true;
    render(
      <CoordinatorHeader
        coordinator={coordinator()}
        coordinators={[coordinator()]}
        workspaceId="ws-1"
        view="needs-you"
        canManage
      />,
    );
    const configureLink = screen.getByRole("link", { name: "Configure" });
    expect(configureLink.className).not.toContain("min-h-11");
    expect(configureLink.className).not.toContain("min-w-11");
  });
});
