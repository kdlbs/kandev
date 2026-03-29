"use client";

import { useRef, useState, useCallback, useEffect } from "react";
import { IconMinus } from "@tabler/icons-react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { PassthroughTerminal } from "./passthrough-terminal";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";

const MIN_HEIGHT = 150;
const MAX_HEIGHT_RATIO = 0.6;
const DEFAULT_HEIGHT = 300;
const STORAGE_KEY_HEIGHT = "bottom-terminal-height";
const STORAGE_KEY_OPEN = "bottom-terminal-open";

type Props = {
  sessionId: string | null;
};

export function BottomTerminalPanel({ sessionId }: Props) {
  const visible = useAppStore((s) => s.bottomTerminal.isOpen);
  const pendingCommand = useAppStore((s) => s.bottomTerminal.pendingCommand);
  const storeApi = useAppStoreApi();
  const toggle = useCallback(() => storeApi.getState().toggleBottomTerminal(), [storeApi]);
  const clearCommand = useCallback(
    () => storeApi.getState().clearBottomTerminalCommand(),
    [storeApi],
  );

  // Restore visibility state from localStorage on mount
  useEffect(() => {
    const saved = localStorage.getItem(STORAGE_KEY_OPEN);
    if (saved === "true" && !storeApi.getState().bottomTerminal.isOpen) {
      toggle();
    }
  }, [storeApi, toggle]);
  const panelRef = useRef<HTMLDivElement>(null);
  const [height, setHeight] = useState(() => {
    if (typeof window === "undefined") return DEFAULT_HEIGHT;
    const saved = localStorage.getItem(STORAGE_KEY_HEIGHT);
    return saved ? Math.max(MIN_HEIGHT, parseInt(saved, 10) || DEFAULT_HEIGHT) : DEFAULT_HEIGHT;
  });
  const isDragging = useRef(false);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      isDragging.current = true;
      const startY = e.clientY;
      const startHeight = height;

      const onMouseMove = (moveEvent: MouseEvent) => {
        if (!isDragging.current) return;
        const delta = startY - moveEvent.clientY;
        const maxHeight = window.innerHeight * MAX_HEIGHT_RATIO;
        const newHeight = Math.min(maxHeight, Math.max(MIN_HEIGHT, startHeight + delta));
        if (panelRef.current) panelRef.current.style.height = `${newHeight}px`;
      };

      const onMouseUp = () => {
        isDragging.current = false;
        if (panelRef.current) {
          const finalHeight = panelRef.current.offsetHeight;
          setHeight(finalHeight);
          localStorage.setItem(STORAGE_KEY_HEIGHT, String(finalHeight));
        }
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
      };

      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
    },
    [height],
  );

  if (!visible) return null;

  return (
    <div
      ref={panelRef}
      className={cn("relative flex flex-col border-t border-border bg-background", "shrink-0")}
      style={{ height }}
    >
      {/* Resize handle */}
      <div
        className="absolute top-0 left-0 right-0 h-1 cursor-ns-resize hover:bg-primary/20 z-10"
        onMouseDown={handleMouseDown}
      />
      {/* Header */}
      <div className="flex items-center justify-between px-3 py-1 border-b border-border bg-muted/30 shrink-0">
        <span className="text-xs font-medium text-muted-foreground">Terminal</span>
        <Button variant="ghost" size="icon" className="h-5 w-5 cursor-pointer" onClick={toggle}>
          <IconMinus className="h-3 w-3" />
        </Button>
      </div>
      {/* Terminal content */}
      <div className="flex-1 min-h-0 overflow-hidden">
        <PassthroughTerminal
          sessionId={sessionId}
          mode="shell"
          terminalId="bottom-panel"
          autoFocus
          pendingCommand={pendingCommand}
          onCommandSent={clearCommand}
        />
      </div>
    </div>
  );
}
