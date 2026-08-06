import type { ReactNode } from "react";
import { RepoGroupHeader } from "./review-diff-list-groups";
import type { ReviewFile } from "./types";

type ReviewDiffGroupProps = {
  group: { repositoryName: string; items: ReviewFile[] };
  showRepoHeaders: boolean;
  renderFile: (file: ReviewFile) => ReactNode;
};

export function ReviewDiffGroup({ group, showRepoHeaders, renderFile }: ReviewDiffGroupProps) {
  return (
    <div data-testid="changes-repo-group" data-repository-name={group.repositoryName}>
      {showRepoHeaders && (
        <RepoGroupHeader
          name={group.repositoryName}
          fileCount={group.items.length}
          isSubmodule={
            Boolean(group.repositoryName) && group.items.some((file) => file.is_submodule)
          }
        />
      )}
      {group.items.map((file) => renderFile(file))}
    </div>
  );
}
