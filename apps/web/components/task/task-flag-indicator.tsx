import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskFlagEditorContext } from "./task-flag-editor-provider";
import { TaskFlagPopover } from "./task-flag-popover";
import { TaskFlagTrigger } from "./task-flag-trigger";

export function TaskFlagIndicator({
  taskId,
  note,
  readOnly,
  onOpen,
}: {
  taskId: string;
  note: string;
  readOnly?: boolean;
  onOpen?: (invoker: HTMLButtonElement) => void;
}) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const editor = useTaskFlagEditorContext();
  const touch = isMobile || !isFinePointer;
  if (touch && onOpen) {
    return (
      <TaskFlagTrigger
        aria-haspopup="dialog"
        aria-description={note}
        onClick={(event) => onOpen(event.currentTarget)}
      />
    );
  }
  if (!touch && !readOnly && editor) {
    return <TaskFlagPopover taskId={taskId} note={note} editor={editor} />;
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <TaskFlagTrigger />
      </TooltipTrigger>
      <TooltipContent className="whitespace-pre-wrap wrap-anywhere">{note}</TooltipContent>
    </Tooltip>
  );
}
