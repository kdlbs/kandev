import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { WatchesNoneNotice, watchesNoBoard } from "./watches-none-notice";

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
});
