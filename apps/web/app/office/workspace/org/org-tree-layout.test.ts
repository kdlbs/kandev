import { describe, expect, it } from "vitest";
import type { AgentProfile } from "@/lib/state/slices/office/types";
import {
  buildForest,
  layoutForest,
  flattenForest,
  collectEdges,
  CARD_W,
  CARD_H,
  GAP_X,
  GAP_Y,
} from "./org-tree-layout";

function makeAgent(id: string, name: string, reportsTo?: string): AgentProfile {
  return {
    id,
    workspaceId: "ws-1",
    name,
    role: "worker",
    status: "idle",
    reportsTo,
    budgetMonthlyCents: 0,
    maxConcurrentSessions: 1,
    agentId: "claude",
    agentDisplayName: "Claude",
    model: "claude-sonnet-4-5",
    allowIndexing: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  };
}

describe("buildForest", () => {
  it("returns agents with no reportsTo as roots", () => {
    const agents = [makeAgent("a", "Alice"), makeAgent("b", "Bob")];
    const roots = buildForest(agents);
    expect(roots).toHaveLength(2);
    expect(roots[0].agent.id).toBe("a");
    expect(roots[1].agent.id).toBe("b");
  });

  it("nests children under their parent", () => {
    const agents = [
      makeAgent("ceo", "CEO"),
      makeAgent("eng", "Engineer", "ceo"),
      makeAgent("design", "Designer", "ceo"),
    ];
    const roots = buildForest(agents);
    expect(roots).toHaveLength(1);
    expect(roots[0].children).toHaveLength(2);
    expect(roots[0].children[0].agent.id).toBe("eng");
    expect(roots[0].children[1].agent.id).toBe("design");
  });

  it("treats agents with missing parent as roots", () => {
    const agents = [makeAgent("a", "Alice", "missing-id"), makeAgent("b", "Bob")];
    const roots = buildForest(agents);
    expect(roots).toHaveLength(2);
  });

  it("builds multi-level hierarchy", () => {
    const agents = [
      makeAgent("ceo", "CEO"),
      makeAgent("vp", "VP", "ceo"),
      makeAgent("eng", "Engineer", "vp"),
    ];
    const roots = buildForest(agents);
    expect(roots).toHaveLength(1);
    expect(roots[0].children[0].children[0].agent.id).toBe("eng");
  });
});

describe("layoutForest", () => {
  it("positions a single root at origin", () => {
    const agents = [makeAgent("a", "Alice")];
    const roots = buildForest(agents);
    layoutForest(roots);
    expect(roots[0].x).toBe(0);
    expect(roots[0].y).toBe(0);
  });

  it("positions children below parent", () => {
    const agents = [makeAgent("ceo", "CEO"), makeAgent("eng", "Engineer", "ceo")];
    const roots = buildForest(agents);
    layoutForest(roots);
    const child = roots[0].children[0];
    expect(child.y).toBe(CARD_H + GAP_Y);
  });

  it("spreads multiple children horizontally", () => {
    const agents = [
      makeAgent("ceo", "CEO"),
      makeAgent("a", "A", "ceo"),
      makeAgent("b", "B", "ceo"),
    ];
    const roots = buildForest(agents);
    layoutForest(roots);
    const [childA, childB] = roots[0].children;
    expect(childB.x).toBeGreaterThan(childA.x);
    expect(childB.x - childA.x).toBe(CARD_W + GAP_X);
  });

  it("positions multiple roots side by side", () => {
    const agents = [makeAgent("a", "A"), makeAgent("b", "B")];
    const roots = buildForest(agents);
    layoutForest(roots);
    expect(roots[1].x).toBeGreaterThan(roots[0].x);
  });
});

describe("flattenForest", () => {
  it("returns all nodes in a flat array", () => {
    const agents = [
      makeAgent("ceo", "CEO"),
      makeAgent("eng", "Engineer", "ceo"),
      makeAgent("design", "Designer", "ceo"),
    ];
    const roots = buildForest(agents);
    layoutForest(roots);
    const flat = flattenForest(roots);
    expect(flat).toHaveLength(3);
  });
});

describe("collectEdges", () => {
  it("returns edges from parent to children", () => {
    const agents = [
      makeAgent("ceo", "CEO"),
      makeAgent("eng", "Engineer", "ceo"),
      makeAgent("design", "Designer", "ceo"),
    ];
    const roots = buildForest(agents);
    layoutForest(roots);
    const edges = collectEdges(roots);
    expect(edges).toHaveLength(2);
    for (const edge of edges) {
      expect(edge.parentY).toBe(CARD_H);
      expect(edge.childY).toBe(CARD_H + GAP_Y);
    }
  });

  it("returns no edges for a single node", () => {
    const agents = [makeAgent("a", "A")];
    const roots = buildForest(agents);
    layoutForest(roots);
    expect(collectEdges(roots)).toHaveLength(0);
  });

  it("provides edge coordinates suitable for L-shaped paths", () => {
    const agents = [makeAgent("ceo", "CEO"), makeAgent("eng", "Engineer", "ceo")];
    const roots = buildForest(agents);
    layoutForest(roots);
    const edges = collectEdges(roots);
    expect(edges).toHaveLength(1);

    const edge = edges[0];
    // Parent center-bottom
    expect(edge.parentX).toBe(roots[0].x + CARD_W / 2);
    expect(edge.parentY).toBe(roots[0].y + CARD_H);
    // Child center-top
    expect(edge.childX).toBe(roots[0].children[0].x + CARD_W / 2);
    expect(edge.childY).toBe(roots[0].children[0].y);

    // Verify L-shape midpoint is between parent bottom and child top
    const midY = (edge.parentY + edge.childY) / 2;
    expect(midY).toBeGreaterThan(edge.parentY);
    expect(midY).toBeLessThan(edge.childY);

    // Verify the L-shaped path string format
    const PADDING = 40;
    const px = edge.parentX + PADDING;
    const py = edge.parentY + PADDING;
    const cx = edge.childX + PADDING;
    const cy = edge.childY + PADDING;
    const pathMidY = (py + cy) / 2;
    const d = `M ${px} ${py} L ${px} ${pathMidY} L ${cx} ${pathMidY} L ${cx} ${cy}`;
    expect(d).toContain("M ");
    expect(d).toContain("L ");
    // Should have 4 points: M start + 3 L segments forming the L-shape
    // split(" L ") gives [M-part, seg1, seg2, seg3] = 4 parts
    expect(d.split(" L ")).toHaveLength(4);
  });
});
