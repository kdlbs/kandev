"use client";

import { useMemo, useState, useCallback, useRef } from "react";
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
  const svgRef = useRef<SVGSVGElement>(null);

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

  const handleExport = useCallback(() => {
    if (!svgRef.current) return;
    const serializer = new XMLSerializer();
    const svgStr = serializer.serializeToString(svgRef.current);
    const blob = new Blob([svgStr], { type: "image/svg+xml;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "org-chart.svg";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, []);

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
      <OrgZoomControls onZoomIn={handleZoomIn} onZoomOut={handleZoomOut} onFit={handleFit} onExport={handleExport} />

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
            ref={svgRef}
            className="absolute inset-0 pointer-events-none"
            width={canvasW}
            height={canvasH}
            xmlns="http://www.w3.org/2000/svg"
            data-testid="org-chart-edges"
          >
            {edges.map((edge, i) => {
              const px = edge.parentX + PADDING;
              const py = edge.parentY + PADDING;
              const cx = edge.childX + PADDING;
              const cy = edge.childY + PADDING;
              const midY = (py + cy) / 2;
              return (
                <path
                  key={i}
                  d={`M ${px} ${py} L ${px} ${midY} L ${cx} ${midY} L ${cx} ${cy}`}
                  fill="none"
                  className="stroke-border"
                  strokeWidth={1.5}
                  data-testid="org-edge"
                />
              );
            })}
          </svg>

          {nodes.map((node) => (
            <OrgNodeCard key={node.agent.id} node={node} />
          ))}
        </div>
      </div>
    </div>
  );
}
