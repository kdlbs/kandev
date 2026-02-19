"use client";

import { useState } from "react";
import { Button } from "@kandev/ui/button";
import { IconArrowBackUp, IconLoader2 } from "@tabler/icons-react";

interface HunkActionBarProps {
  changeBlockId: string;
  onRevert: () => Promise<void> | void;
  onMouseEnter: () => void;
  onMouseLeave: () => void;
}

export function HunkActionBar({
  changeBlockId,
  onRevert,
  onMouseEnter,
  onMouseLeave,
}: HunkActionBarProps) {
  const [loading, setLoading] = useState(false);

  const handleClick = async () => {
    setLoading(true);
    try {
      await onRevert();
      // Don't setLoading(false) on success — the diff will re-render
      // and unmount this component when the git status refreshes
    } catch {
      setLoading(false);
    }
  };

  return (
    <div
      data-cb={changeBlockId}
      style={{
        position: "relative",
        zIndex: 10,
        width: "100%",
        overflow: "visible",
      }}
    >
      <div
        data-undo-btn=""
        style={{ opacity: 0, pointerEvents: "none", transition: "opacity 150ms" }}
        className="absolute top-1 right-2"
        onMouseEnter={onMouseEnter}
        onMouseLeave={onMouseLeave}
      >
        <Button
          variant="ghost"
          size="sm"
          disabled={loading}
          className="h-6 gap-1 px-2 text-xs cursor-pointer rounded border border-border/60 bg-background text-muted-foreground shadow-sm hover:text-red-500 hover:bg-red-500/10 hover:border-red-500/30"
          onClick={handleClick}
        >
          {loading ? (
            <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <IconArrowBackUp className="h-3.5 w-3.5" />
          )}
          Undo
        </Button>
      </div>
    </div>
  );
}
