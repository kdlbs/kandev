import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { UnknownCoordinatorState } from "./unknown-coordinator-state";

afterEach(cleanup);

describe("UnknownCoordinatorState", () => {
  it("shows the message and a link to the coordinators list in settings", () => {
    render(<UnknownCoordinatorState workspaceId="ws-1" />);
    expect(screen.getByText("This coordinator is not in this workspace.")).not.toBeNull();
    expect(screen.getByRole("link", { name: "See coordinators" }).getAttribute("href")).toBe(
      "/settings/workspaces/ws-1/coordinators",
    );
  });
});
