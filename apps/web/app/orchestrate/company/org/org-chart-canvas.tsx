"use client";

import { useMemo, useState, useCallback } from "react";
import type { AgentInstance } from "@/lib/state/slices/orchestrate/types";
import { OrgNodeCard } from "./org-node-card";
import { OrgZoomControls } from "./org-zoom-controls";
import {
  buildForest,
  layoutForest,
  flattenForest,
  collectEdges,
  CARD_W,
  CARD_H,
} from "./org-tree-layout";

type OrgChartCanvasProps = {
  agents: AgentInstance[];
};

const ZOOM_STEP = 0.15;
const ZOOM_MIN = 0.3;
const ZOOM_MAX = 2.0;
const PADDING = 40;

export function OrgChartCanvas({ agents }: OrgChartCanvasProps) {
  const [zoom, setZoom] = useState(1);

  const { nodes, edges, canvasW, canvasH } = useMemo(() => {
    const roots = buildForest(agents);
    layoutForest(roots);
    const allNodes = flattenForest(roots);
    const allEdges = collectEdges(roots);

    let maxX = 0;
    let maxY = 0;
    for (const n of allNodes) {
      maxX = Math.max(maxX, n.x + CARD_W);
      maxY = Math.max(maxY, n.y + CARD_H);
    }

    return {
      nodes: allNodes,
      edges: allEdges,
      canvasW: maxX + PADDING * 2,
      canvasH: maxY + PADDING * 2,
    };
  }, [agents]);

  const handleZoomIn = useCallback(() => {
    setZoom((z) => Math.min(ZOOM_MAX, z + ZOOM_STEP));
  }, []);

  const handleZoomOut = useCallback(() => {
    setZoom((z) => Math.max(ZOOM_MIN, z - ZOOM_STEP));
  }, []);

  const handleFit = useCallback(() => setZoom(1), []);

  if (agents.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-center">
        <p className="text-sm text-muted-foreground">No agents to display.</p>
        <p className="text-xs text-muted-foreground mt-1">
          Create agents and set their reporting structure to see the org chart.
        </p>
      </div>
    );
  }

  return (
    <div className="relative flex-1 min-h-0 overflow-auto">
      <OrgZoomControls onZoomIn={handleZoomIn} onZoomOut={handleZoomOut} onFit={handleFit} />

      <div
        style={{
          width: canvasW * zoom,
          height: canvasH * zoom,
          transform: `scale(${zoom})`,
          transformOrigin: "top left",
          minWidth: canvasW,
          minHeight: canvasH,
        }}
      >
        <div className="relative" style={{ padding: PADDING }}>
          <svg
            className="absolute inset-0 pointer-events-none"
            width={canvasW}
            height={canvasH}
          >
            {edges.map((edge, i) => (
              <line
                key={i}
                x1={edge.parentX + PADDING}
                y1={edge.parentY + PADDING}
                x2={edge.childX + PADDING}
                y2={edge.childY + PADDING}
                className="stroke-border"
                strokeWidth={1.5}
              />
            ))}
          </svg>

          {nodes.map((node) => (
            <OrgNodeCard key={node.agent.id} node={node} />
          ))}
        </div>
      </div>
    </div>
  );
}
