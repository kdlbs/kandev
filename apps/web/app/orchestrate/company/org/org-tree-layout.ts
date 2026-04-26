import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";

export const CARD_W = 200;
export const CARD_H = 100;
export const GAP_X = 32;
export const GAP_Y = 80;

export type OrgTreeNode = {
  agent: AgentInstance;
  children: OrgTreeNode[];
  x: number;
  y: number;
  width: number;
};

/**
 * Build a forest of OrgTreeNode from a flat list of AgentInstance.
 * Agents with no `reportsTo` (or whose parent is missing) become roots.
 */
export function buildForest(agents: AgentInstance[]): OrgTreeNode[] {
  const nodeMap = new Map<string, OrgTreeNode>();
  for (const agent of agents) {
    nodeMap.set(agent.id, { agent, children: [], x: 0, y: 0, width: 0 });
  }
  const roots: OrgTreeNode[] = [];
  for (const agent of agents) {
    const node = nodeMap.get(agent.id)!;
    const parent = agent.reportsTo ? nodeMap.get(agent.reportsTo) : undefined;
    if (parent) {
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }
  return roots;
}

/** Compute the total horizontal width a subtree needs. */
export function subtreeWidth(node: OrgTreeNode): number {
  if (node.children.length === 0) {
    node.width = CARD_W;
    return CARD_W;
  }
  let total = 0;
  for (const child of node.children) {
    total += subtreeWidth(child);
  }
  total += GAP_X * (node.children.length - 1);
  node.width = Math.max(CARD_W, total);
  return node.width;
}

/** Recursively assign x,y positions to each node in the tree. */
export function layoutTree(node: OrgTreeNode, x: number, y: number): void {
  node.x = x + (node.width - CARD_W) / 2;
  node.y = y;

  if (node.children.length === 0) return;

  let childX = x;
  for (const child of node.children) {
    subtreeWidth(child);
    layoutTree(child, childX, y + CARD_H + GAP_Y);
    childX += child.width + GAP_X;
  }
}

/** Layout multiple root trees side by side, returning all positioned nodes. */
export function layoutForest(roots: OrgTreeNode[]): OrgTreeNode[] {
  for (const root of roots) {
    subtreeWidth(root);
  }

  let offsetX = 0;
  for (const root of roots) {
    layoutTree(root, offsetX, 0);
    offsetX += root.width + GAP_X * 2;
  }

  return roots;
}

/** Flatten a forest into a flat list of all nodes. */
export function flattenForest(roots: OrgTreeNode[]): OrgTreeNode[] {
  const result: OrgTreeNode[] = [];
  function walk(node: OrgTreeNode) {
    result.push(node);
    for (const child of node.children) {
      walk(child);
    }
  }
  for (const root of roots) {
    walk(root);
  }
  return result;
}

/** Collect all parent-child edges for SVG line drawing. */
export type Edge = {
  parentX: number;
  parentY: number;
  childX: number;
  childY: number;
};

export function collectEdges(roots: OrgTreeNode[]): Edge[] {
  const edges: Edge[] = [];
  function walk(node: OrgTreeNode) {
    for (const child of node.children) {
      edges.push({
        parentX: node.x + CARD_W / 2,
        parentY: node.y + CARD_H,
        childX: child.x + CARD_W / 2,
        childY: child.y,
      });
      walk(child);
    }
  }
  for (const root of roots) {
    walk(root);
  }
  return edges;
}
