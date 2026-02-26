"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useTheme } from "next-themes";
import { IconZoomIn, IconZoomOut, IconCode } from "@tabler/icons-react";
import { DEFAULT_SCALE, SCALE_STEP, MIN_SCALE, MAX_SCALE, getSvgDimensions } from "./mermaid-utils";

type MermaidAPI = typeof import("mermaid").default;

let mermaidPromise: Promise<MermaidAPI> | null = null;
let mermaidIdCounter = 0;

function getMermaid(): Promise<MermaidAPI> {
  if (!mermaidPromise) {
    mermaidPromise = import("mermaid").then((m) => m.default);
  }
  return mermaidPromise;
}

type MermaidBlockProps = {
  code: string;
};

export function MermaidBlock({ code }: MermaidBlockProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<string | null>(null);
  const [scale, setScale] = useState(DEFAULT_SCALE);
  const [svgSize, setSvgSize] = useState<{ w: number; h: number } | null>(null);
  const [showCode, setShowCode] = useState(false);
  const { resolvedTheme } = useTheme();

  useEffect(() => {
    if (!code.trim()) return;

    let cancelled = false;
    const theme = resolvedTheme === "dark" ? "dark" : "default";
    const id = `mermaid-md-${++mermaidIdCounter}`;

    getMermaid()
      .then((mermaid) => {
        mermaid.initialize({ startOnLoad: false, theme, securityLevel: "loose" });
        return mermaid.render(id, code);
      })
      .then(({ svg }) => {
        if (!cancelled && containerRef.current) {
          containerRef.current.innerHTML = svg;
          setSvgSize(getSvgDimensions(containerRef.current));
          setError(null);
        }
      })
      .catch((err: Error) => {
        if (!cancelled) setError(err.message);
      });

    return () => {
      cancelled = true;
    };
  }, [code, resolvedTheme]);

  const zoomIn = useCallback(() => setScale((s) => Math.min(s + SCALE_STEP, MAX_SCALE)), []);
  const zoomOut = useCallback(() => setScale((s) => Math.max(s - SCALE_STEP, MIN_SCALE)), []);
  const zoomReset = useCallback(() => setScale(DEFAULT_SCALE), []);
  const toggleCode = useCallback(() => setShowCode((v) => !v), []);

  if (error) {
    return (
      <div className="my-3 rounded-md border border-destructive/30 bg-destructive/5 p-3">
        <p className="text-xs text-destructive mb-2">Failed to render diagram</p>
        <pre className="text-xs whitespace-pre-wrap font-mono text-muted-foreground">{code}</pre>
      </div>
    );
  }

  // Wrapper clips to scaled dimensions; container keeps intrinsic size so transform works
  const wrapperStyle: React.CSSProperties = svgSize
    ? { width: svgSize.w * scale, height: svgSize.h * scale, overflow: "hidden" }
    : {};
  const containerStyle: React.CSSProperties = {
    transformOrigin: "top left",
    transform: `scale(${scale})`,
    ...(svgSize ? { width: svgSize.w, height: svgSize.h } : {}),
  };

  return (
    <div className="mermaid-block group/mermaid relative my-3 rounded-md border border-border/50 bg-muted/20 inline-block">
      <div className="overflow-auto p-3" style={{ display: showCode ? "none" : undefined }}>
        <div style={wrapperStyle}>
          <div ref={containerRef} style={containerStyle} />
        </div>
      </div>
      {showCode && (
        <pre className="m-0 p-3 text-xs leading-relaxed whitespace-pre-wrap font-mono text-muted-foreground bg-transparent">
          <code>{code}</code>
        </pre>
      )}
      <MermaidToolbar
        showCode={showCode}
        scale={scale}
        onZoomIn={zoomIn}
        onZoomOut={zoomOut}
        onReset={zoomReset}
        onToggleCode={toggleCode}
      />
    </div>
  );
}

type MermaidToolbarProps = {
  showCode?: boolean;
  scale?: number;
  onZoomIn?: () => void;
  onZoomOut?: () => void;
  onReset?: () => void;
  onToggleCode: () => void;
};

function MermaidToolbar({
  showCode,
  scale,
  onZoomIn,
  onZoomOut,
  onReset,
  onToggleCode,
}: MermaidToolbarProps) {
  return (
    <div className="absolute top-1.5 right-1.5 flex items-center gap-1 px-1.5 py-0.5 rounded-md bg-background/80 border border-border/50 backdrop-blur-sm opacity-0 group-hover/mermaid:opacity-100 transition-opacity z-10">
      {!showCode && onZoomOut && onReset && onZoomIn && scale != null && (
        <>
          <button
            type="button"
            onClick={onZoomOut}
            className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
          >
            <IconZoomOut className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={onReset}
            className="text-[10px] text-muted-foreground hover:text-foreground tabular-nums min-w-[3ch] text-center cursor-pointer"
          >
            {Math.round(scale * 100)}%
          </button>
          <button
            type="button"
            onClick={onZoomIn}
            className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
          >
            <IconZoomIn className="h-3.5 w-3.5" />
          </button>
          <div className="w-px h-3.5 bg-border/50 mx-0.5" />
        </>
      )}
      <button
        type="button"
        onClick={onToggleCode}
        className="p-0.5 text-muted-foreground hover:text-foreground cursor-pointer"
        title={showCode ? "Show diagram" : "Show code"}
      >
        <IconCode className="h-3.5 w-3.5" />
      </button>
    </div>
  );
}
