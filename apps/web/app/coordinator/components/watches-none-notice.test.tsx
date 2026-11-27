import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WatchesNoneNotice, watchesNoBoard, watchesNoProject } from "./watches-none-notice";

afterEach(cleanup);

const empty = { scope: "selected" as const, workflowIds: [] };

describe("WatchesNoneNotice", () => {
  it("is empty only for a selected scope with no board", () => {
    expect(watchesNoBoard(empty)).toBe(true);
    expect(watchesNoBoard({ scope: "all", workflowIds: [] })).toBe(false);
    expect(watchesNoBoard({ scope: "selected", workflowIds: ["a"] })).toBe(false);
    expect(watchesNoBoard(undefined)).toBe(false);
  });

  it("renders nothing while boards are watched", () => {
    render(
      <WatchesNoneNotice
        workspaceId="w"
        coordinatorId="c"
        watchSet={{ scope: "all", workflowIds: [] }}
      />,
    );
    expect(screen.queryByTestId("watches-none-notice")).toBeNull();
  });

  it("links a manager to the Watches section", () => {
    render(<WatchesNoneNotice workspaceId="w" coordinatorId="c" watchSet={empty} chooseBoards />);
    expect(screen.getByText("This coordinator watches no board.")).toBeTruthy();
    expect(screen.getByTestId("watches-none-choose").getAttribute("href")).toBe(
      "/settings/workspaces/w/coordinators/c?section=watches",
    );
  });

  it("is informational for a reader and selects the section in place on Configure", () => {
    const { rerender } = render(
      <WatchesNoneNotice workspaceId="w" coordinatorId="c" watchSet={empty} />,
    );
    expect(screen.queryByTestId("watches-none-choose")).toBeNull();
    const choose = vi.fn();
    rerender(
      <WatchesNoneNotice
        workspaceId="w"
        coordinatorId="c"
        watchSet={empty}
        onChooseBoards={choose}
      />,
    );
    fireEvent.click(screen.getByTestId("watches-none-choose"));
    expect(choose).toHaveBeenCalled();
  });

  it("is empty of projects only for a known empty selection without the no-repository toggle", () => {
    const projects = (repositoryIds: string[] | null, includeNoRepository: boolean) => ({
      scope: "all" as const,
      workflowIds: [],
      projects: { scope: "selected" as const, repositoryIds, includeNoRepository },
    });
    expect(watchesNoProject(projects([], false))).toBe(true);
    expect(watchesNoProject(projects([], true))).toBe(false);
    expect(watchesNoProject(projects(["r"], false))).toBe(false);
    expect(watchesNoProject(projects(null, false))).toBe(false);
    expect(watchesNoProject({ scope: "all", workflowIds: [] })).toBe(false);
  });

  it("says the coordinator watches no project and offers Choose projects", () => {
    const choose = vi.fn();
    render(
      <WatchesNoneNotice
        workspaceId="w"
        coordinatorId="c"
        watchSet={{
          scope: "all",
          workflowIds: [],
          projects: { scope: "selected", repositoryIds: [], includeNoRepository: false },
        }}
        onChooseBoards={choose}
      />,
    );
    expect(screen.getByText(/watches no project/)).toBeTruthy();
    expect(screen.getByText("Choose projects")).toBeTruthy();
  });
});
