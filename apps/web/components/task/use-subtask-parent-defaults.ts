import { useEffect, useRef } from "react";
import type { DialogFormState } from "@/components/task-create-dialog-types";

export function useSubtaskParentDefaults({
  parentRepositoryId,
  baseBranch,
  defaultProfileId,
  repositoriesDirty,
  agentProfileId,
  setRepositories,
  setAgentProfileId,
}: Pick<
  DialogFormState,
  "repositoriesDirty" | "agentProfileId" | "setRepositories" | "setAgentProfileId"
> & {
  parentRepositoryId: string | null;
  baseBranch: string | null;
  defaultProfileId: string;
}) {
  const repositorySeeded = useRef(false);
  const profileSeeded = useRef(false);
  useEffect(() => {
    if (repositorySeeded.current || repositoriesDirty || !parentRepositoryId) return;
    repositorySeeded.current = true;
    setRepositories([
      { key: "subtask-row-1", repositoryId: parentRepositoryId, branch: baseBranch ?? "" },
    ]);
  }, [parentRepositoryId, baseBranch, repositoriesDirty, setRepositories]);
  useEffect(() => {
    if (profileSeeded.current || agentProfileId || !defaultProfileId) return;
    profileSeeded.current = true;
    setAgentProfileId(defaultProfileId);
  }, [defaultProfileId, agentProfileId, setAgentProfileId]);
}
