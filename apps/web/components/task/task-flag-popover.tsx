import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import { useHoverPopover } from "@/components/integrations/use-hover-popover";
import type { useTaskFlagEditorContext } from "./task-flag-editor-provider";
import { TaskFlagForm, useTaskFlagDraft } from "./task-flag-editor";
import { stopTaskFlagEvent, TaskFlagTrigger } from "./task-flag-trigger";

type FlagPopoverProps = {
  taskId: string;
  note: string;
  editor: NonNullable<ReturnType<typeof useTaskFlagEditorContext>>;
};

function useFlagPopover({ taskId, note, editor }: FlagPopoverProps) {
  const anchor = useRef<HTMLButtonElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const invoker = useRef<HTMLElement | null>(null);
  const restoreFocus = useRef(false);
  const restoringFocus = useRef(false);
  const focusOnOpen = useRef(false);
  const [open, setOpen] = useState(false);
  const openRef = useRef(false);

  const close = () => {
    restoreFocus.current =
      Boolean(invoker.current) || Boolean(content.current?.contains(document.activeElement));
    openRef.current = false;
    setOpen(false);
  };

  const draft = useTaskFlagDraft(note, (next) => editor.save(taskId, next), close);
  const retryPending = useRef(false);
  retryPending.current = draft.failed;
  const { reset, submitting } = draft;
  const setHoverOpen = useCallback(
    (next: boolean) => {
      if (!next && (submitting.current || retryPending.current)) return;
      if (next && !openRef.current) {
        reset(note);
      }
      openRef.current = next;
      setOpen(next);
    },
    [note, reset, submitting],
  );
  const hover = useHoverPopover({
    open,
    onOpenChange: setHoverOpen,
    openDelayMs: 120,
    closeDelayMs: 200,
  });
  const { onOpenChange } = hover;

  const focusEditor = useCallback(
    (from: HTMLElement | null) => {
      invoker.current = from;
      onOpenChange(true);
      const input = content.current?.querySelector("textarea");
      if (input) input.focus();
      else focusOnOpen.current = true;
    },
    [onOpenChange],
  );

  useEffect(() => {
    if (anchor.current) return editor.registerPopover(anchor.current, focusEditor);
  }, [editor, focusEditor]);

  const onOpenAutoFocus = (event: Event) => {
    event.preventDefault();
    if (focusOnOpen.current) content.current?.querySelector("textarea")?.focus();
    focusOnOpen.current = false;
  };

  const onCloseAutoFocus = (event: Event) => {
    event.preventDefault();
    const origin = invoker.current;
    invoker.current = null;
    if (!restoreFocus.current && (!origin || anchor.current?.isConnected)) return;
    restoreFocus.current = false;
    restoringFocus.current = true;
    editor.restoreFocus(origin, anchor.current);
    restoringFocus.current = false;
  };

  const onTriggerFocus = (event: React.FocusEvent<HTMLButtonElement>) => {
    if (!restoringFocus.current) {
      hover.onTriggerEnter(event);
      onOpenChange(true);
    }
  };

  return {
    anchor,
    content,
    open,
    hover,
    draft,
    focusEditor,
    onOpenAutoFocus,
    onCloseAutoFocus,
    onTriggerFocus,
  };
}

export function TaskFlagPopover(props: FlagPopoverProps) {
  const { t } = useTranslation();
  const popoverId = useId();
  const surface = useFlagPopover(props);
  const { anchor, content, open, hover, draft, focusEditor } = surface;

  return (
    <Popover open={open} onOpenChange={hover.onOpenChange}>
      <PopoverAnchor asChild>
        <TaskFlagTrigger
          ref={anchor}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-controls={open ? popoverId : undefined}
          aria-description={props.note}
          onPointerEnter={hover.onTriggerEnter}
          onPointerLeave={hover.onTriggerLeave}
          onFocus={surface.onTriggerFocus}
          onBlur={hover.onTriggerLeave}
          onClick={(event) => focusEditor(event.currentTarget)}
          onKeyDown={(event) => {
            if (open && event.key === "Tab" && !event.shiftKey) {
              event.preventDefault();
              focusEditor(event.currentTarget);
            }
          }}
        />
      </PopoverAnchor>
      <PopoverContent
        id={popoverId}
        ref={content}
        aria-label={t("sidebar:editFlag")}
        align="start"
        side="right"
        collisionPadding={8}
        className="max-h-(--radix-popover-content-available-height) w-80 max-w-[calc(100vw-1rem)] overflow-y-auto overscroll-contain p-3"
        onPointerEnter={hover.onContentEnter}
        onPointerLeave={hover.onContentLeave}
        onFocus={hover.onContentEnter}
        onBlur={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget)) hover.onContentLeave(event);
        }}
        onInteractOutside={(event) => {
          if (event.target instanceof Node && anchor.current?.contains(event.target)) {
            event.preventDefault();
          }
        }}
        onOpenAutoFocus={surface.onOpenAutoFocus}
        onCloseAutoFocus={surface.onCloseAutoFocus}
        onEscapeKeyDown={(event) => {
          event.preventDefault();
          draft.close();
        }}
        onKeyDown={stopTaskFlagEvent}
        onClick={stopTaskFlagEvent}
        onPointerDown={stopTaskFlagEvent}
        onMouseDown={stopTaskFlagEvent}
        onTouchStart={stopTaskFlagEvent}
        onContextMenu={stopTaskFlagEvent}
      >
        <TaskFlagForm draft={draft} />
      </PopoverContent>
    </Popover>
  );
}
