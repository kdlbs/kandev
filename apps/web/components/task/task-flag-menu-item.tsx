import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconFlag, IconPencil, IconTrash } from "@tabler/icons-react";
import { ContextMenuItem, ContextMenuSub, ContextMenuSubTrigger } from "@kandev/ui/context-menu";
import { useToast } from "@/components/toast-provider";
import { useSaveTaskFlag } from "@/hooks/use-save-task-flag";
import { TaskContextMenuSubContent } from "./task-context-menu-sub-content";
import type { TaskSwitcherItem } from "./task-switcher-types";

export type TaskFlagMenuActions = {
  edit: () => void;
  remove: () => Promise<void>;
  removing: boolean;
};

export function useTaskFlagRemoval(taskId: string) {
  const [removing, setRemoving] = useState(false);
  const removingRef = useRef(false);
  const saveTaskFlag = useSaveTaskFlag();
  const { t } = useTranslation();
  const { toast } = useToast();

  const remove = async () => {
    if (removingRef.current) return;
    removingRef.current = true;
    setRemoving(true);
    try {
      await saveTaskFlag(taskId, "");
    } catch {
      toast({ variant: "error", title: t("sidebar:flagSaveFailed") });
    } finally {
      removingRef.current = false;
      setRemoving(false);
    }
  };

  return { removing, remove };
}

const flagActionSize = "max-md:min-h-11 [@media(pointer:coarse)]:min-h-11";

export function TaskFlagItem({
  task,
  disabled,
  actions,
}: {
  task: TaskSwitcherItem;
  disabled?: boolean;
  actions?: TaskFlagMenuActions;
}) {
  const { t } = useTranslation();
  if (task.isArchived || !actions) return null;
  const unavailable = disabled || task.isPendingRemoval || actions.removing;
  const icon = <IconFlag className="mr-2 h-4 w-4" aria-hidden="true" />;

  if (!task.flagNote) {
    return (
      <ContextMenuItem className={flagActionSize} disabled={unavailable} onSelect={actions.edit}>
        {icon}
        {t("sidebar:addFlag")}
      </ContextMenuItem>
    );
  }

  return (
    <ContextMenuSub>
      <ContextMenuSubTrigger className={flagActionSize} disabled={unavailable}>
        {icon}
        {t("sidebar:changeFlag")}
      </ContextMenuSubTrigger>
      <TaskContextMenuSubContent className="w-40">
        <ContextMenuItem className={flagActionSize} onSelect={actions.edit}>
          <IconPencil className="mr-2 h-4 w-4" aria-hidden="true" />
          {t("sidebar:editFlag")}
        </ContextMenuItem>
        <ContextMenuItem
          className={flagActionSize}
          variant="destructive"
          onSelect={() => void actions.remove()}
        >
          <IconTrash className="mr-2 h-4 w-4" aria-hidden="true" />
          {t("sidebar:removeFlag")}
        </ContextMenuItem>
      </TaskContextMenuSubContent>
    </ContextMenuSub>
  );
}
