import { useEffect, useRef, useState } from "react";
import type { DialogFormState } from "@/components/task-create-dialog-types";

export function useSubtaskParentDefaults({
  parentRepositoryId,
  baseBranch,
  defaultProfileId,
  repositoriesDirty,
  agentProfileId,
  setRepositories,
  setAgentProfileId,
  hasRepositorySelection = false,
  onAgentProfileChange = setAgentProfileId,
}: Pick<
  DialogFormState,
  "repositoriesDirty" | "agentProfileId" | "setRepositories" | "setAgentProfileId"
> & {
  parentRepositoryId: string | null;
  baseBranch: string | null;
  defaultProfileId: string;
  hasRepositorySelection?: boolean;
  onAgentProfileChange?: (value: string) => void;
}) {
  const repositorySeeded = useRef(false);
  const profileSeeded = useRef(false);
  const [profileTouched, setProfileTouched] = useState(false);
  useEffect(() => {
    if (repositorySeeded.current || !parentRepositoryId) return;
    repositorySeeded.current = true;
    if (repositoriesDirty || hasRepositorySelection) return;
    setRepositories([
      { key: "subtask-row-1", repositoryId: parentRepositoryId, branch: baseBranch ?? "" },
    ]);
  }, [parentRepositoryId, baseBranch, repositoriesDirty, hasRepositorySelection, setRepositories]);
  useEffect(() => {
    if (profileSeeded.current || !defaultProfileId) return;
    profileSeeded.current = true;
    if (!agentProfileId) setAgentProfileId(defaultProfileId);
  }, [defaultProfileId, agentProfileId, setAgentProfileId]);
  return {
    submissionDefaultProfileId: profileTouched ? "" : defaultProfileId,
    onChange: (value: string) => {
      profileSeeded.current = true;
      setProfileTouched(true);
      onAgentProfileChange(value);
    },
  };
}
