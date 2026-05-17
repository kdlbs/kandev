"use client";

import { IconClick } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";

interface InspectButtonProps {
  active: boolean;
  disabled?: boolean;
  count?: number;
  onToggle: () => void;
}

export function InspectButton({ active, disabled, count = 0, onToggle }: InspectButtonProps) {
  const title = active
    ? "Inspecting - click to pin, drag to select an area, Esc to exit"
    : "Inspect: click to pin or drag to select an area of the preview";
  return (
    <Button
      size="sm"
      variant={active ? "default" : "outline"}
      onClick={onToggle}
      disabled={disabled}
      className="cursor-pointer relative"
      data-testid="preview-inspect-button"
      aria-pressed={active}
      aria-label={active ? "Exit inspect mode" : "Enter inspect mode"}
      title={title}
    >
      <IconClick className="h-4 w-4" />
      {count > 0 && (
        <span data-testid="preview-inspect-count" className="ml-1 text-xs font-mono">
          {count}
        </span>
      )}
    </Button>
  );
}
