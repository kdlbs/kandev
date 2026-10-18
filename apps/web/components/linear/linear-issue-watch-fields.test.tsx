import { cleanup, fireEvent, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectMultiSelect, useTeamsAndStates } from "./linear-issue-watch-fields";

const responsive = vi.hoisted(() => ({ isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

const api = vi.hoisted(() => ({
  listLinearTeams: vi.fn(async () => ({ teams: [] })),
  listLinearStates: vi.fn(async () => ({ states: [] })),
  listLinearLabels: vi.fn(async () => ({ labels: [] })),
  listLinearUsers: vi.fn(async () => ({ users: [] })),
  listLinearProjects: vi.fn(async (_teamKey: string, opts?: { workspaceId?: string }) => ({
    projects: [{ id: `${opts?.workspaceId}-project`, name: `${opts?.workspaceId} project` }],
  })),
}));

vi.mock("@/lib/api/domains/linear-api", () => api);

const projects = [
  { id: "p1", name: "Checkout" },
  { id: "p2", name: "Search" },
];

afterEach(() => {
  cleanup();
  responsive.isFinePointer = true;
});

describe("ProjectMultiSelect", () => {
  // @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
  it("uses 44px hit areas on coarse pointers and compact chips on fine pointers", () => {
    responsive.isFinePointer = false;
    const { rerender } = render(
      <ProjectMultiSelect
        projects={projects}
        loading={false}
        selected={[]}
        onToggle={vi.fn()}
        disabled={false}
      />,
    );
    expect(screen.getByRole("button", { name: "Checkout" }).className).toContain("min-h-11");

    responsive.isFinePointer = true;
    rerender(
      <ProjectMultiSelect
        projects={projects}
        loading={false}
        selected={[]}
        onToggle={vi.fn()}
        disabled={false}
      />,
    );
    expect(screen.getByRole("button", { name: "Checkout" }).className).not.toContain("min-h-11");
  });

  // @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
  it("keeps a selected project the team no longer lists removable", () => {
    const onToggle = vi.fn();
    render(
      <ProjectMultiSelect
        projects={[]}
        loading={false}
        selected={["p-archived"]}
        onToggle={onToggle}
        disabled={false}
      />,
    );
    const chip = screen.getByRole("button", { name: "Unavailable project" });
    expect(chip.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(chip);
    expect(onToggle).toHaveBeenCalledWith("p-archived");
  });
});

describe("useTeamsAndStates", () => {
  // @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
  it("loads projects per workspace when two workspaces share a team key", async () => {
    const { result, rerender } = renderHook(
      ({ workspaceId }) => useTeamsAndStates(workspaceId, "ENG"),
      { initialProps: { workspaceId: "ws-a" } },
    );
    await waitFor(() =>
      expect(result.current.projects).toEqual([{ id: "ws-a-project", name: "ws-a project" }]),
    );

    rerender({ workspaceId: "ws-b" });
    expect(result.current.projects).toEqual([]);
    await waitFor(() =>
      expect(result.current.projects).toEqual([{ id: "ws-b-project", name: "ws-b project" }]),
    );
    expect(api.listLinearProjects).toHaveBeenCalledWith("ENG", { workspaceId: "ws-a" });
    expect(api.listLinearProjects).toHaveBeenCalledWith("ENG", { workspaceId: "ws-b" });
  });
});
