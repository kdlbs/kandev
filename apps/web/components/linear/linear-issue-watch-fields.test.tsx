import { useState } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectMultiSelect, useTeamsAndStates } from "./linear-issue-watch-fields";
import { FilterFields } from "./linear-issue-watch-filter-fields";
import { makeEmptyForm, type FormState } from "./linear-issue-watch-form";

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

const archivedProjectId = "p-archived";

const projects = [
  { id: "p1", name: "Checkout" },
  { id: "p2", name: "Search" },
];

afterEach(() => {
  cleanup();
  responsive.isFinePointer = true;
  vi.clearAllMocks();
});

function WatchFilters({ initial }: { initial: Partial<FormState> }) {
  const [form, setForm] = useState({ ...makeEmptyForm("ws-a"), ...initial });
  return (
    <>
      <FilterFields form={form} setForm={setForm} />
      <output data-testid="filter-state">{JSON.stringify(form)}</output>
    </>
  );
}

describe("watch dialog filter fields", () => {
  // @covers AC-INTEGRATIONS-LINEAR-PROJECTS-001.3
  it("keeps project-only saved selections visible and removable without a team", async () => {
    render(
      <WatchFilters
        initial={{
          projectIds: [archivedProjectId, "p-other"],
          query: "keep query",
          priorities: [1],
        }}
      />,
    );
    await waitFor(() => expect(api.listLinearTeams).toHaveBeenCalled());
    expect(screen.queryByTitle(archivedProjectId)).not.toBeNull();
    const chip = screen.getByTitle(archivedProjectId);
    expect(chip.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(chip);
    expect(JSON.parse(screen.getByTestId("filter-state").textContent!)).toMatchObject({
      teamKey: "",
      projectIds: ["p-other"],
      query: "keep query",
      priorities: [1],
    });
    fireEvent.click(screen.getByTitle("p-other"));
    expect(JSON.parse(screen.getByTestId("filter-state").textContent!).projectIds).toEqual([]);
    expect(screen.queryByTestId("linear-watch-projects")).toBeNull();
    expect(api.listLinearProjects).not.toHaveBeenCalled();
  });

  it("keeps the no-team helper and hides inventory when nothing is selected", async () => {
    render(<WatchFilters initial={{}} />);
    await waitFor(() => expect(api.listLinearTeams).toHaveBeenCalled());
    expect(screen.getByText("Pick a team to choose specific projects.")).toBeTruthy();
    expect(screen.queryByTestId("linear-watch-projects")).toBeNull();
    expect(api.listLinearProjects).not.toHaveBeenCalled();
  });

  it("shows team loading before offering the returned projects", async () => {
    let finish!: (value: { projects: Array<{ id: string; name: string }> }) => void;
    api.listLinearProjects.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    render(<WatchFilters initial={{ teamKey: "ENG" }} />);
    expect(screen.getByText("Loading projects…")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Checkout" })).toBeNull();
    await act(async () => finish({ projects: [{ id: "p1", name: "Checkout" }] }));
    expect(screen.getByRole("button", { name: "Checkout" }).getAttribute("aria-pressed")).toBe(
      "false",
    );
    expect(api.listLinearProjects).toHaveBeenCalledWith("ENG", { workspaceId: "ws-a" });
  });
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
        selected={[archivedProjectId]}
        onToggle={onToggle}
        disabled={false}
      />,
    );
    const chip = screen.getByRole("button", { name: "Unavailable project" });
    expect(chip.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(chip);
    expect(onToggle).toHaveBeenCalledWith(archivedProjectId);
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
