"use client";

import { IconPlus, IconMinus, IconArrowsMaximize, IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

type OrgZoomControlsProps = {
  onZoomIn: () => void;
  onZoomOut: () => void;
  onFit: () => void;
  onExport?: () => void;
};

export function OrgZoomControls({ onZoomIn, onZoomOut, onFit, onExport }: OrgZoomControlsProps) {
  return (
    <div className="absolute top-4 right-4 z-10 flex flex-col gap-1">
      <Button
        variant="outline"
        size="icon"
        className="h-8 w-8 cursor-pointer"
        onClick={onZoomIn}
      >
        <IconPlus className="h-4 w-4" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        className="h-8 w-8 cursor-pointer"
        onClick={onZoomOut}
      >
        <IconMinus className="h-4 w-4" />
      </Button>
      <Button
        variant="outline"
        size="sm"
        className="h-8 cursor-pointer"
        onClick={onFit}
      >
        <IconArrowsMaximize className="h-4 w-4" />
      </Button>
      {onExport && (
        <Button
          variant="outline"
          size="sm"
          className="h-8 cursor-pointer"
          onClick={onExport}
        >
          <IconDownload className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}
