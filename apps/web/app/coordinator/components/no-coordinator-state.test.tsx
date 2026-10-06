import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { NoCoordinatorState } from "./no-coordinator-state";

afterEach(cleanup);

describe("NoCoordinatorState", () => {
  it("shows the body copy with no action for a reader", () => {
    render(<NoCoordinatorState workspaceId="ws-1" canManage={false} />);
    expect(
      screen.getByText(
        "No coordinator in this workspace yet. Questions from your agents still wait on their tasks.",
      ),
    ).not.toBeNull();
    expect(screen.queryByText("Add a coordinator")).toBeNull();
  });

  it("shows Add a coordinator for a manager, linking to the add page", () => {
    render(<NoCoordinatorState workspaceId="ws-1" canManage />);
    expect(screen.getByRole("link", { name: "Add a coordinator" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/coordinators/new",
    );
  });
});
