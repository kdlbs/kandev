"use client";

import type { ReactNode } from "react";
import {
  IconClick,
  IconMessageDots,
  IconPointer,
  IconScreenshot,
  IconTextScan2,
  type Icon,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { annotationDoneButtonClass } from "@/components/shared/region-annotation/annotation-mode-hint";
import type { PreviewAnnotationTool } from "@/lib/preview-annotation";
import { cn } from "@/lib/utils";

type ToolOption = { tool: PreviewAnnotationTool; icon: Icon; label: string; hint: string };

function useToolOptions(): ToolOption[] {
  const { t } = useTranslation();
  return [
    {
      tool: "interact",
      icon: IconPointer,
      label: t("task:previewToolInteract"),
      hint: t("task:previewToolInteractHint"),
    },
    {
      tool: "text",
      icon: IconTextScan2,
      label: t("task:previewToolText"),
      hint: t("task:previewToolTextHint"),
    },
    {
      tool: "element",
      icon: IconClick,
      label: t("task:previewToolElement"),
      hint: t("task:previewToolElementHint"),
    },
    {
      tool: "region",
      icon: IconScreenshot,
      label: t("task:previewToolRegion"),
      hint: t("task:previewToolRegionHint"),
    },
  ];
}

type Props = {
  touch: boolean;
  tool: PreviewAnnotationTool;
  acknowledgedMode: string | null;
  pendingCount: number;
  regionDisabled: boolean;
  onToolChange: (tool: PreviewAnnotationTool) => void;
  onExit: () => void;
  /** Wraps the count button, so the host can anchor the pending panel to it. */
  renderCount: (button: ReactNode) => ReactNode;
  onOpenPending: () => void;
};

function ToolButton({
  option,
  pressed,
  disabled,
  touch,
  onSelect,
}: {
  option: ToolOption;
  pressed: boolean;
  disabled: boolean;
  touch: boolean;
  onSelect: () => void;
}) {
  const Icon = option.icon;
  const button = (
    <Button
      type="button"
      size="sm"
      variant={pressed ? "default" : "ghost"}
      aria-pressed={pressed}
      aria-label={option.label}
      disabled={disabled}
      data-testid={`preview-tool-${option.tool}`}
      className={cn(
        "cursor-pointer gap-1 rounded-full",
        touch ? "h-11 min-w-11 px-2" : "h-7 px-2 text-xs",
      )}
      onClick={onSelect}
    >
      <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
      {!touch && <span>{option.label}</span>}
    </Button>
  );
  if (touch) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={disabled ? 0 : -1} className="inline-flex">
          {button}
        </span>
      </TooltipTrigger>
      <TooltipContent>{option.hint}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Annotation tools that stay selected across captures, the pending count, and
 * Exit. Floats at the top on fine pointers; a bottom bar on coarse pointers.
 */
export function PreviewAnnotationToolbar(props: Props) {
  const { t } = useTranslation();
  const options = useToolOptions();
  const { touch } = props;
  const countButton = (
    <Button
      type="button"
      size="sm"
      variant="ghost"
      className={cn("cursor-pointer gap-1 rounded-full", touch ? "h-11 min-w-11 px-2" : "h-7 px-2")}
      aria-label={t("task:previewFeedbackCount", { count: props.pendingCount })}
      data-testid="preview-annotation-count"
      onClick={props.onOpenPending}
    >
      <IconMessageDots className="h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="text-xs font-medium tabular-nums">{props.pendingCount}</span>
    </Button>
  );
  return (
    <div
      role="toolbar"
      aria-label={t("task:previewAnnotationToolbar")}
      data-testid="preview-annotation-toolbar"
      data-capture-mode={props.acknowledgedMode ?? "none"}
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      className={cn(
        "pointer-events-auto absolute z-20 flex items-center gap-1 border bg-background/95 shadow-sm",
        touch
          ? "inset-x-0 bottom-0 justify-between border-x-0 border-b-0 px-2 pt-1 pb-[max(0.25rem,env(safe-area-inset-bottom))]"
          : "top-2 left-1/2 -translate-x-1/2 rounded-full p-1",
      )}
    >
      <TooltipProvider>
        <div className="flex items-center gap-0.5">
          {options.map((option) => (
            <ToolButton
              key={option.tool}
              option={option}
              touch={touch}
              pressed={props.tool === option.tool}
              disabled={option.tool === "region" && props.regionDisabled}
              onSelect={() => props.onToolChange(option.tool)}
            />
          ))}
        </div>
      </TooltipProvider>
      <div className="flex items-center gap-1">
        <span aria-hidden="true" className="mx-0.5 h-5 w-px bg-border" />
        {props.renderCount(countButton)}
        <Button
          type="button"
          size="sm"
          variant="outline"
          className={annotationDoneButtonClass}
          aria-label={t("task:previewExitAnnotationLabel")}
          data-testid="preview-annotation-exit"
          onClick={props.onExit}
        >
          {t("task:previewExitAnnotation")}
        </Button>
      </div>
    </div>
  );
}
