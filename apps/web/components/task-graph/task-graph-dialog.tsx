import { lazy, Suspense, type ReactNode, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import { Spinner } from "@kandev/ui/spinner";

// Lazy so the graph libraries load only when the dialog opens.
const TaskGraphContent = lazy(() =>
  import("./task-graph-content").then((m) => ({ default: m.TaskGraphContent })),
);

export function TaskGraphDialog({
  taskId,
  open,
  onOpenChange,
  trigger,
  returnFocusTo,
}: {
  taskId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Radix restores focus only to a DialogTrigger, so openers pass one... */
  trigger?: ReactNode;
  /** ...or name the element that should regain focus on close. */
  returnFocusTo?: RefObject<HTMLElement | null>;
}) {
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {trigger && <DialogTrigger asChild>{trigger}</DialogTrigger>}
      <DialogContent
        onCloseAutoFocus={(event) => {
          const target = returnFocusTo?.current;
          if (!target?.isConnected) return;
          event.preventDefault();
          target.focus();
        }}
        data-testid="task-graph-dialog"
        showCloseButton={false}
        enterConfirms={false}
        className="flex h-dvh w-full max-w-full flex-col gap-3 overflow-hidden rounded-none p-3 sm:max-w-none md:h-[min(800px,90vh)] md:w-[min(1200px,95vw)] md:max-w-none md:rounded-lg md:p-4"
      >
        <DialogHeader className="flex-row items-center justify-between gap-2">
          <DialogTitle className="text-sm">{t("task:taskGraphTitle")}</DialogTitle>
          <DialogDescription className="sr-only">
            {t("task:taskGraphDescription")}
          </DialogDescription>
          <DialogClose asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t("common:close")}
              data-testid="task-graph-close"
              className="cursor-pointer [@media(pointer:coarse)]:size-11"
            >
              <IconX />
            </Button>
          </DialogClose>
        </DialogHeader>
        {open && (
          <Suspense
            fallback={
              <div className="flex flex-1 items-center justify-center">
                <Spinner aria-hidden="true" />
              </div>
            }
          >
            <TaskGraphContent taskId={taskId} onNavigate={() => onOpenChange(false)} />
          </Suspense>
        )}
      </DialogContent>
    </Dialog>
  );
}
