import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { ClassifyResult } from "@/lib/coordinator/attention";
import { CountStrip } from "./count-strip";

afterEach(cleanup);

function classification(overrides: Partial<ClassifyResult> = {}): ClassifyResult {
  return {
    needsYou: [],
    queue: { working: [], in_review: [], ready_to_merge: [], done: [], other: [] },
    ...overrides,
  } as ClassifyResult;
}

describe("CountStrip", () => {
  it("shows the four counts from the classification", () => {
    render(
      <CountStrip
        classification={classification({
          needsYou: [{ id: "1" } as ClassifyResult["needsYou"][number]],
        })}
        workspaceId="ws-1"
        coordinatorId="co-1"
      />,
    );
    expect(screen.getByTestId("count-needs-you").textContent).toContain("1");
    expect(screen.getByTestId("count-working").textContent).toContain("0");
    expect(
      screen.getByText("Positions derived from session, PR, CI and review facts, as they change"),
    ).not.toBeNull();
  });

  it("links the Needs you count to Needs you and the others to their Queue group", () => {
    render(
      <CountStrip classification={classification()} workspaceId="ws-1" coordinatorId="co-1" />,
    );
    expect(screen.getByTestId("count-needs-you").getAttribute("href")).toBe(
      "/workspaces/ws-1/coordinator/co-1",
    );
    expect(screen.getByTestId("count-working").getAttribute("href")).toBe(
      "/workspaces/ws-1/coordinator/co-1/queue?group=working",
    );
    expect(screen.getByTestId("count-in-review").getAttribute("href")).toBe(
      "/workspaces/ws-1/coordinator/co-1/queue?group=in_review",
    );
    expect(screen.getByTestId("count-ready-to-merge").getAttribute("href")).toBe(
      "/workspaces/ws-1/coordinator/co-1/queue?group=ready_to_merge",
    );
  });
});
