import { defaultSubtaskWorkspaceMode, type SubtaskWorkspaceMode } from "./new-subtask-form-state";
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

export function useInitialWorkspaceMode(
  worktreeBranch: string | null,
  contextReady: boolean,
  sourceTouched: boolean,
) {
  const [mode, setMode] = useState<SubtaskWorkspaceMode>(() =>
    defaultSubtaskWorkspaceMode(worktreeBranch),
  );
  const seededOrTouched = useRef(false);
  useEffect(() => {
    if (seededOrTouched.current) return;
    if (sourceTouched) {
      seededOrTouched.current = true;
      return;
    }
    if (!contextReady) return;
    seededOrTouched.current = true;
    setMode(defaultSubtaskWorkspaceMode(worktreeBranch));
  }, [contextReady, worktreeBranch, sourceTouched]);
  const onChange = (value: SubtaskWorkspaceMode) => {
    seededOrTouched.current = true;
    setMode(value);
  };
  return [mode, onChange] as const;
}
