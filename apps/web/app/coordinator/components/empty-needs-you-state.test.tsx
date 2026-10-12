import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { EmptyNeedsYouState } from "./empty-needs-you-state";

afterEach(cleanup);

describe("EmptyNeedsYouState", () => {
  it("shows the working count and a link to Queue's working group", () => {
    render(<EmptyNeedsYouState workingCount={3} workspaceId="ws-1" coordinatorId="co-1" />);
    expect(screen.getByText("Nothing needs you. That is the working state.")).not.toBeNull();
    expect(screen.getByText("3 cards are running.")).not.toBeNull();
    expect(screen.getByRole("link", { name: "See what is running" }).getAttribute("href")).toBe(
      "/workspaces/ws-1/coordinator/co-1/queue?group=working",
    );
  });

  it("uses the singular form for one card", () => {
    render(<EmptyNeedsYouState workingCount={1} workspaceId="ws-1" coordinatorId="co-1" />);
    expect(screen.getByText("1 card is running.")).not.toBeNull();
  });
});
