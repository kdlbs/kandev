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

const NEEDS_YOU_TEST_ID = "count-needs-you";
const ARIA_CURRENT = "aria-current";
const CURRENT = "page";
const QUEUE_TEST_IDS = ["count-working", "count-in-review", "count-ready-to-merge"];

describe("CountStrip", () => {
  it("shows the four counts from the classification", () => {
    render(
      <CountStrip
        classification={classification({
          needsYou: [{ id: "1" } as ClassifyResult["needsYou"][number]],
        })}
        workspaceId="ws-1"
        coordinatorId="co-1"
        view="needs-you"
      />,
    );
    expect(screen.getByTestId(NEEDS_YOU_TEST_ID).textContent).toContain("1");
    expect(screen.getByTestId("count-working").textContent).toContain("0");
    expect(
      screen.getByText("Positions derived from session, PR, CI and review facts, as they change"),
    ).not.toBeNull();
  });

  it("links the Needs you count to Needs you and the others to their Queue group", () => {
    render(
      <CountStrip
        classification={classification()}
        workspaceId="ws-1"
        coordinatorId="co-1"
        view="needs-you"
      />,
    );
    expect(screen.getByTestId(NEEDS_YOU_TEST_ID).getAttribute("href")).toBe(
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

  it("marks only Needs you as current on the Needs you screen", () => {
    render(
      <CountStrip
        classification={classification()}
        workspaceId="ws-1"
        coordinatorId="co-1"
        view="needs-you"
      />,
    );
    expect(screen.getByTestId(NEEDS_YOU_TEST_ID).getAttribute(ARIA_CURRENT)).toBe(CURRENT);
    for (const testId of QUEUE_TEST_IDS) {
      expect(screen.getByTestId(testId).getAttribute(ARIA_CURRENT)).toBeNull();
    }
  });

  it("marks all three Queue cells as current together on the Queue screen", () => {
    render(
      <CountStrip
        classification={classification()}
        workspaceId="ws-1"
        coordinatorId="co-1"
        view="queue"
      />,
    );
    for (const testId of QUEUE_TEST_IDS) {
      expect(screen.getByTestId(testId).getAttribute(ARIA_CURRENT)).toBe(CURRENT);
    }
    expect(screen.getByTestId(NEEDS_YOU_TEST_ID).getAttribute(ARIA_CURRENT)).toBeNull();
  });

  it("gives the current cells the selected background and the others a hover background", () => {
    render(
      <CountStrip
        classification={classification()}
        workspaceId="ws-1"
        coordinatorId="co-1"
        view="queue"
      />,
    );
    for (const testId of QUEUE_TEST_IDS) {
      expect(screen.getByTestId(testId).className).toContain("bg-primary/10");
    }
    expect(screen.getByTestId(NEEDS_YOU_TEST_ID).className).toContain("hover:bg-accent");
    expect(screen.getByTestId(NEEDS_YOU_TEST_ID).className).not.toContain("bg-primary/10");
  });
});
