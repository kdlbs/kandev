import { useState } from "react";
import type { Repository } from "@/lib/types/http";
import type { TaskCreateDialogProps } from "@/components/task-create-dialog";
import type {
  DialogFormState,
  useTaskCreateDialogData,
} from "@/components/task-create-dialog-state";
import { useRepositorySets } from "@/hooks/domains/workspace/use-repository-sets";
import { useApplyRepositorySet } from "@/components/task-create-dialog-repository-sets-apply";
import { selectedRepositoryIdsForSet } from "@/components/task-create-dialog-repository-sets";

export function useRepositorySetsForTaskCreateDialog(
  props: TaskCreateDialogProps,
  fs: DialogFormState,
  repositories: Repository[],
  computed: ReturnType<typeof useTaskCreateDialogData>["computed"],
  userSettingsLoaded: boolean,
) {
  const { sets } = useRepositorySets(props.workspaceId ?? null, props.open);
  const onApply = useApplyRepositorySet({
    rows: fs.repositories,
    repositories,
    setRepositories: fs.setRepositories,
    setRepositoriesDirty: fs.setRepositoriesDirty,
  });
  const [saveOpen, setSaveOpen] = useState(false);
  const canSave =
    Boolean(props.workspaceId) && selectedRepositoryIdsForSet(fs.repositories).length > 0;

  if (!userSettingsLoaded) return undefined;
  return {
    sets,
    onApply,
    save:
      canSave && props.workspaceId
        ? {
            workspaceId: props.workspaceId,
            rows: fs.repositories,
            repositories,
            isLocalExecutor: computed.isLocalExecutor,
            freshBranchEnabled: fs.freshBranchEnabled,
            open: saveOpen,
            setOpen: setSaveOpen,
          }
        : null,
  };
}
