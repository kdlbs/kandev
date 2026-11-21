"use client";

import type { ReactNode, Ref } from "react";
import {
  IconClick,
  IconGripVertical,
  IconLayoutSidebarRight,
  IconPointer,
  IconScreenshot,
  IconTextScan2,
  type Icon,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";
import { annotationDoneButtonClass } from "@/components/shared/region-annotation/annotation-mode-hint";
import type { usePreviewToolbarPosition } from "@/hooks/use-preview-toolbar-position";
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
  /** Wraps the panel toggle, so the host can anchor a sheet to it. */
  renderCount: (button: ReactNode) => ReactNode;
  panelOpen: boolean;
  onTogglePanel: () => void;
  ref?: Ref<HTMLDivElement>;
  /** Where the user placed the toolbar; absent on coarse pointers, where it is the bottom bar. */
  position?: ReturnType<typeof usePreviewToolbarPosition>;
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
 * The accented panel toggle. Returned as a plain button element, so a host
 * can make it the trigger of its comment sheet.
 */
function panelToggle({
  touch,
  open,
  count,
  label,
  onToggle,
}: {
  touch: boolean;
  open: boolean;
  count: number;
  label: string;
  onToggle: () => void;
}) {
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className={cn(
        "cursor-pointer gap-1 rounded-full border-primary text-primary hover:bg-primary/10 hover:text-primary",
        open && "bg-primary/15",
        touch ? "h-11 min-w-11 px-2" : "h-7 px-2",
      )}
      aria-pressed={open}
      title={label}
      data-testid="preview-annotation-count"
      onClick={onToggle}
    >
      <IconLayoutSidebarRight className="h-4 w-4 shrink-0" aria-hidden="true" />
      <span className="sr-only">{label}</span>
      <span className="text-xs font-semibold tabular-nums">{count}</span>
    </Button>
  );
}

function DragHandle({ position }: { position: NonNullable<Props["position"]> }) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      aria-label={t("task:previewMoveToolbar")}
      title={t("task:previewMoveToolbarHint")}
      data-testid="preview-annotation-toolbar-handle"
      className="flex h-7 w-5 shrink-0 cursor-grab items-center justify-center rounded-full text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none active:cursor-grabbing"
      {...position.handleProps}
    >
      <IconGripVertical className="h-4 w-4" aria-hidden="true" />
    </button>
  );
}

/**
 * Annotation tools that stay selected across captures, the panel toggle, and
 * Exit. Floats over the preview on fine pointers, where the user can drag it;
 * a bottom bar on coarse pointers.
 */
export function PreviewAnnotationToolbar(props: Props) {
  const { t } = useTranslation();
  const options = useToolOptions();
  const { touch, position } = props;
  return (
    <div
      ref={props.ref}
      role="toolbar"
      aria-label={t("task:previewAnnotationToolbar")}
      data-testid="preview-annotation-toolbar"
      data-capture-mode={props.acknowledgedMode ?? "none"}
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      style={position?.style}
      className={cn(
        "pointer-events-auto absolute z-40 flex items-center gap-1 bg-background",
        touch
          ? "inset-x-0 bottom-0 justify-between border-t px-2 pt-1 pb-[max(0.25rem,env(safe-area-inset-bottom))]"
          : "rounded-full p-1 shadow-lg ring-2 ring-primary",
        !touch && !position && "top-2 left-1/2 -translate-x-1/2",
      )}
    >
      {position && <DragHandle position={position} />}
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
        {props.renderCount(
          panelToggle({
            touch,
            open: props.panelOpen,
            count: props.pendingCount,
            label: props.panelOpen ? t("task:previewHideComments") : t("task:previewShowComments"),
            onToggle: props.onTogglePanel,
          }),
        )}
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
