"use client";

import { useRef, type FocusEvent, type MouseEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskSubtasks, type TaskSubtask } from "@/hooks/domains/kanban/use-task-subtasks";
import { useHoverPopover } from "@/components/integrations/use-hover-popover";
import { cn } from "@/lib/utils";
import { TaskSubtaskRow } from "./task-subtask-row";

const MAX_VISIBLE_SUBTASKS = 12;
const OPEN_DELAY_MS = 200;
const CLOSE_DELAY_MS = 100;

function SubtasksSection({ subtasks }: { subtasks: TaskSubtask[] }) {
  const { t } = useTranslation();
  if (subtasks.length === 0) return null;

  const visible = subtasks.slice(0, MAX_VISIBLE_SUBTASKS);
  const overflowCount = subtasks.length - visible.length;

  return (
    <div
      className="mt-2.5 border-t border-border/60 pt-2.5"
      data-testid="task-title-hover-subtasks"
    >
      <div className="text-[11px] font-medium text-muted-foreground">
        {t("task:subtasksHeading", { count: subtasks.length })}
      </div>
      <div className="mt-1.5 space-y-0.5">
        {visible.map((subtask) => (
          <TaskSubtaskRow key={subtask.id} subtask={subtask} />
        ))}
        {overflowCount > 0 && (
          <div className="px-1 py-1 text-xs text-muted-foreground">
            {t("task:moreNotShown", { count: overflowCount })}
          </div>
        )}
      </div>
    </div>
  );
}

function DesktopTaskTitlePreview({
  title,
  children,
  subtasks,
  side,
  align,
  triggerClassName,
}: {
  title: string;
  children: ReactNode;
  subtasks: TaskSubtask[];
  side: "top" | "right" | "bottom" | "left";
  align: "start" | "center" | "end";
  triggerClassName?: string;
}) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const keyboardSessionRef = useRef(false);
  const hover = useHoverPopover({ openDelayMs: OPEN_DELAY_MS, closeDelayMs: CLOSE_DELAY_MS });

  const handleTriggerClick = (event: MouseEvent<HTMLButtonElement>) => {
    if (event.detail !== 0) return;
    event.preventDefault();
    event.stopPropagation();
    keyboardSessionRef.current = true;
    hover.onOpenChange(true);
  };

  const handleContentBlur = (event: FocusEvent<HTMLDivElement>) => {
    if (!event.currentTarget.contains(event.relatedTarget)) hover.onContentLeave();
  };

  return (
    <Popover open={hover.open} onOpenChange={hover.onOpenChange}>
      <PopoverAnchor asChild>
        <button
          ref={triggerRef}
          type="button"
          data-testid="task-title-preview-trigger"
          aria-haspopup="dialog"
          aria-expanded={hover.open}
          onPointerEnter={hover.onTriggerEnter}
          onPointerLeave={hover.onTriggerLeave}
          onKeyDown={(event) => {
            // Only swallow keys while the preview is open (protects it from
            // the card/row's own keyboard shortcuts, e.g. drag pickup).
            // Escape must always fall through to close the surface it sits
            // on (kanban preview panel, dialog, ...) even when the trigger
            // merely has focus from a plain click and the preview isn't open.
            if (!hover.open && event.key === "Escape") return;
            event.stopPropagation();
          }}
          onClick={handleTriggerClick}
          className={cn(
            "min-w-0 max-w-full cursor-pointer text-left outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
            triggerClassName,
          )}
        >
          {children}
        </button>
      </PopoverAnchor>
      <PopoverContent
        ref={contentRef}
        side={side}
        align={align}
        data-testid="task-title-hover-card"
        className="w-80 max-w-[calc(100vw-1rem)] max-h-80 overflow-y-auto p-3"
        onPointerEnter={hover.onContentEnter}
        onPointerLeave={() => {
          if (!contentRef.current?.contains(document.activeElement)) hover.onContentLeave();
        }}
        onFocusCapture={hover.onContentEnter}
        onBlurCapture={handleContentBlur}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          if (keyboardSessionRef.current) {
            contentRef.current
              ?.querySelector<HTMLElement>(
                'a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])',
              )
              ?.focus();
          }
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          if (keyboardSessionRef.current) triggerRef.current?.focus();
          keyboardSessionRef.current = false;
        }}
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
      >
        <div className="text-pretty break-words text-sm font-semibold leading-snug text-foreground [overflow-wrap:anywhere]">
          {title}
        </div>
        <SubtasksSection subtasks={subtasks} />
      </PopoverContent>
    </Popover>
  );
}

/** Interactive desktop preview with direct mobile navigation as the coarse-pointer fallback. */
export function TaskTitleHoverCard({
  taskId,
  title,
  children,
  side = "bottom",
  align = "start",
  triggerClassName,
}: {
  taskId: string;
  title: string;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
  triggerClassName?: string;
}) {
  const { isFinePointer } = useResponsiveBreakpoint();
  const subtasks = useTaskSubtasks(taskId);

  if (!isFinePointer || subtasks.length === 0) return <>{children}</>;

  return (
    <DesktopTaskTitlePreview
      title={title}
      subtasks={subtasks}
      side={side}
      align={align}
      triggerClassName={triggerClassName}
    >
      {children}
    </DesktopTaskTitlePreview>
  );
}
