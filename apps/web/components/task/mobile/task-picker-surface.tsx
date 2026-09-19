import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronRight, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent } from "@kandev/ui/sheet";
import { TaskSwitcherDrawer } from "./task-switcher-drawer";

export function InlineTaskHeader({
  expanded,
  onExpandedChange,
  onNewTask,
}: {
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  onNewTask: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 border-t border-border pt-4">
      <Button
        variant="ghost"
        className="h-11 min-w-11 flex-1 justify-start gap-2 px-0 text-sm font-medium hover:bg-transparent aria-expanded:bg-transparent"
        aria-expanded={expanded}
        aria-controls="mobile-navigation-task-body"
        data-testid="mobile-navigation-tasks-toggle"
        onClick={() => onExpandedChange(!expanded)}
      >
        {t("task:tasks")}
        {expanded ? (
          <IconChevronDown className="size-3.5 text-muted-foreground" />
        ) : (
          <IconChevronRight className="size-3.5 text-muted-foreground" />
        )}
      </Button>
      <Button
        variant="ghost"
        className="size-11 shrink-0 text-muted-foreground hover:text-foreground"
        onClick={onNewTask}
        aria-label={t("task:newTask")}
      >
        <IconPlus className="size-4" />
      </Button>
    </div>
  );
}

export function TaskPickerSurface({
  presentation,
  workspaceId,
  open,
  onOpenChange,
  onCloseAutoFocus,
  children,
}: {
  presentation?: "sheet" | "drawer";
  workspaceId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCloseAutoFocus?: (event: Event) => void;
} & { children: ReactNode }) {
  if (presentation === "drawer")
    return (
      <TaskSwitcherDrawer
        key={workspaceId}
        open={open}
        onOpenChange={onOpenChange}
        onCloseAutoFocus={onCloseAutoFocus}
      >
        {children}
      </TaskSwitcherDrawer>
    );
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        onCloseAutoFocus={onCloseAutoFocus}
        showCloseButton={false}
        side="left"
        className="w-[85vw] max-w-sm p-0 flex flex-col"
      >
        {children}
      </SheetContent>
    </Sheet>
  );
}
