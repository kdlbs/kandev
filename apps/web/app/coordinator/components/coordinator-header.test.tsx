import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import { CoordinatorConfigureAction, CoordinatorTitleSlot } from "./coordinator-header";

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

const CONFIGURE = "Configure";

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

function renderTitleSlot(coordinators: Coordinator[]) {
  return render(
    <CoordinatorTitleSlot
      coordinator={coordinator()}
      coordinators={coordinators}
      workspaceId="ws-1"
      view="queue"
      title="Queue"
    />,
  );
}

describe("CoordinatorTitleSlot", () => {
  it("names the coordinator and the screen, in that order", () => {
    renderTitleSlot([coordinator()]);
    expect(screen.getByText("Planner")).not.toBeNull();
    expect(screen.getByText("Queue")).not.toBeNull();
  });

  it("links the coordinator crumb to this screen for that coordinator", () => {
    renderTitleSlot([coordinator()]);
    expect(screen.getByTestId("coordinator-crumb").getAttribute("href")).toBe(
      "/workspaces/ws-1/coordinator/co-1/queue",
    );
  });

  it("shows the coordinator's name as a plain crumb with a single coordinator", () => {
    renderTitleSlot([coordinator()]);
    expect(screen.queryByTestId("coordinator-selector")).toBeNull();
  });

  it("shows a selector in place of the crumb with several coordinators", () => {
    renderTitleSlot([coordinator(), coordinator({ id: "co-2", name: "Reviewer" })]);
    expect(screen.getByTestId("coordinator-selector")).not.toBeNull();
    expect(screen.queryByTestId("coordinator-crumb")).toBeNull();
    expect(screen.getByText("Queue")).not.toBeNull();
  });
});

describe("CoordinatorConfigureAction", () => {
  it("links to the coordinator's settings page", () => {
    render(<CoordinatorConfigureAction coordinator={coordinator()} workspaceId="ws-1" />);
    expect(screen.getByRole("link", { name: CONFIGURE }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/coordinators/co-1",
    );
  });

  it("sizes Configure for touch on a coarse pointer", () => {
    isFinePointer = false;
    render(<CoordinatorConfigureAction coordinator={coordinator()} workspaceId="ws-1" />);
    const configureLink = screen.getByRole("link", { name: CONFIGURE });
    expect(configureLink.className).toContain("min-h-11");
    expect(configureLink.className).toContain("min-w-11");
  });

  it("does not force touch sizing for Configure on a fine pointer", () => {
    isFinePointer = true;
    render(<CoordinatorConfigureAction coordinator={coordinator()} workspaceId="ws-1" />);
    const configureLink = screen.getByRole("link", { name: CONFIGURE });
    expect(configureLink.className).not.toContain("min-h-11");
    expect(configureLink.className).not.toContain("min-w-11");
  });
});
