"use client";

import { IconClick } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";

interface InspectButtonProps {
  active: boolean;
  disabled?: boolean;
  count?: number;
  onToggle: () => void;
}

export function InspectButton({ active, disabled, count = 0, onToggle }: InspectButtonProps) {
  const { t } = useTranslation();
  const title = active ? t("task:inspectingClickToPin") : t("task:inspectClickToPinOrDrag");
  return (
    <Button
      size="sm"
      variant={active ? "default" : "outline"}
      onClick={onToggle}
      disabled={disabled}
      className="cursor-pointer relative"
      data-testid="preview-inspect-button"
      aria-pressed={active}
      aria-label={active ? t("task:exitInspectMode") : t("task:enterInspectMode")}
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
