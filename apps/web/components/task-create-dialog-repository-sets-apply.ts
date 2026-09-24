"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";

import { useToast } from "@/components/toast-provider";
import type { Repository, RepositorySet } from "@/lib/types/http";
import type { TaskRepoRow, TaskRepositorySelection } from "@/components/task-create-dialog-types";
import { applyRepositorySet } from "@/components/task-create-dialog-repository-sets";

type UseApplyRepositorySetArgs = {
  rows: TaskRepoRow[];
  repositories: Repository[];
  setRepositories: (rows: TaskRepoRow[]) => void;
  setRepositoriesDirty: (dirty: boolean) => void;
  setNoRepository: (noRepository: boolean) => void;
  selections?: TaskRepositorySelection[];
  setRepositorySelections?: (selections: TaskRepositorySelection[]) => void;
};

/**
 * Turns a chosen repository set into picker rows.
 *
 * Applying a set writes nothing to the server: it only fills the form, so the
 * repositories that reach the task are whatever the form holds at submit. The
 * toast exists because two of the rules are silent otherwise - members already in
 * the form are skipped, and members whose repository no longer exists are dropped
 * - and a user who is not told would read either as the set being wrong.
 */
export function useApplyRepositorySet({
  rows,
  repositories,
  setRepositories,
  setRepositoriesDirty,
  setNoRepository,
  selections,
  setRepositorySelections,
}: UseApplyRepositorySetArgs) {
  const { t } = useTranslation();
  const { toast } = useToast();

  return useCallback(
    (set: RepositorySet) => {
      const outcome = applyRepositorySet({ rows, set, repositories });
      if (outcome.addedCount > 0) {
        setNoRepository(false);
        if (selections && setRepositorySelections) {
          const existingKeys = new Set(selections.map((selection) => selection.key));
          const next = selections.filter(
            (selection) =>
              !(
                selection.kind === "local" &&
                !selection.repositoryId &&
                !selection.localPath &&
                !selection.branch
              ),
          );
          for (const row of outcome.rows) {
            if (!existingKeys.has(row.key)) next.push({ kind: "local", ...row });
          }
          setRepositorySelections(next);
        } else {
          setRepositories(outcome.rows);
        }
        setRepositoriesDirty(true);
      }
      if (outcome.missingCount > 0) {
        toast({
          title: t("task:repositorySetsApplied", {
            count: outcome.addedCount,
            name: set.name,
          }),
          description: t("task:repositorySetsSkippedMissing", {
            count: outcome.missingCount,
            name: set.name,
          }),
          variant: "default",
        });
        return;
      }
      if (outcome.addedCount === 0) {
        toast({
          title: t("task:repositorySetsNothingToApply", { name: set.name }),
          variant: "default",
        });
      }
    },
    [
      rows,
      repositories,
      setRepositories,
      setRepositoriesDirty,
      setNoRepository,
      selections,
      setRepositorySelections,
      t,
      toast,
    ],
  );
}
