import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { NeedsYouAutonomyItem } from "@/lib/coordinator/attention";
import { AutonomyItemCard } from "./autonomy-item-card";

afterEach(cleanup);

function item(over: Partial<NeedsYouAutonomyItem> = {}): NeedsYouAutonomyItem {
  return {
    kind: "autonomy",
    id: "autonomy:c1",
    reason: "ceiling_reached",
    detail: "",
    pendingWakes: 3,
    conditions: [],
    referenceTimeMs: 0,
    ageMs: 3_600_000,
    ...over,
  };
}

function renderCard(i: NeedsYouAutonomyItem, canManage = true) {
  return render(
    <AutonomyItemCard item={i} workspaceId="w1" coordinatorId="c1" canManage={canManage} />,
  );
}

const ITEM_TITLE = "autonomy-item-title";
const ITEM_FIX = "autonomy-item-fix";

describe("AutonomyItemCard", () => {
  it("shows the held reason, the plural why line, the fix text and age", () => {
    renderCard(item());
    expect(screen.getByTestId(ITEM_TITLE).textContent).toBe("Cost ceiling reached");
    expect(screen.getByTestId("autonomy-item-why").textContent).toBe(
      "3 events are waiting for the coordinator",
    );
    expect(screen.getByTestId(ITEM_FIX).textContent).toBe(
      "Spend in the last 24 hours is at the ceiling. Raise the ceiling or wait for spend to age out.",
    );
    expect(screen.getByText("1h 0m")).toBeTruthy();
  });

  it("uses the singular why line for one event", () => {
    renderCard(item({ pendingWakes: 1 }));
    expect(screen.getByTestId("autonomy-item-why").textContent).toBe(
      "1 event is waiting for the coordinator",
    );
  });

  it("offers Open settings to the Autonomy section for a manager only", () => {
    renderCard(item());
    expect(screen.getByTestId("autonomy-item-open-settings").getAttribute("href")).toBe(
      "/settings/workspaces/w1/coordinators/c1?section=autonomy",
    );
    cleanup();
    renderCard(item(), false);
    expect(screen.queryByTestId("autonomy-item-open-settings")).toBeNull();
    expect(screen.queryByText("Open settings")).toBeNull();
  });

  it("uses the condition's fix text for containment, and its detail override when present", () => {
    renderCard(
      item({
        reason: "containment",
        detail: "no_extra_tools",
        conditions: [{ name: "no_extra_tools", met: false, detail: "" }],
      }),
    );
    expect(screen.getByTestId(ITEM_TITLE).textContent).toBe(
      "Containment not in place: No extra MCP servers",
    );
    expect(screen.getByTestId(ITEM_FIX).textContent).toBe(
      "Remove extra MCP servers from this coordinator's agent profile.",
    );
    cleanup();
    renderCard(
      item({
        reason: "containment",
        detail: "no_extra_tools",
        conditions: [{ name: "no_extra_tools", met: false, detail: "unreadable" }],
      }),
    );
    expect(screen.getByTestId(ITEM_FIX).textContent).toBe(
      "Kandev could not read this setting. Check the profile and try again.",
    );
  });

  it("has no fix line when containment.conditions lacks the named condition", () => {
    renderCard(item({ reason: "containment", detail: "auth_enabled", conditions: [] }));
    expect(screen.getByTestId(ITEM_TITLE).textContent).toBe(
      "Containment not in place: Authentication enabled",
    );
    expect(screen.queryByTestId(ITEM_FIX)).toBeNull();
  });

  it("uses the session_not_started wording for conversation_unavailable", () => {
    renderCard(item({ reason: "conversation_unavailable", detail: "session_not_started" }));
    expect(screen.getByTestId(ITEM_TITLE).textContent).toBe(
      "The conversation has not started. Open the copilot to start it",
    );
  });
});
