import { describe, expect, it } from "vitest";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";
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

function makeAgent(id: string, name: string, reportsTo?: string): AgentInstance {
  return {
    id,
    workspaceId: "ws-1",
    name,
    role: "worker",
    status: "idle",
    reportsTo,
    budgetMonthlyCents: 0,
    maxConcurrentSessions: 1,
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
    const agents = [
      makeAgent("a", "Alice", "missing-id"),
      makeAgent("b", "Bob"),
    ];
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
    const agents = [
      makeAgent("ceo", "CEO"),
      makeAgent("eng", "Engineer", "ceo"),
    ];
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
});
