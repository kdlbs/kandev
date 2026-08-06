import type { ReactNode } from "react";
import { RepoGroupHeader } from "./review-diff-list-groups";
import type { ReviewFile } from "./types";

type ReviewDiffGroupProps = {
  group: { repositoryName: string; items: ReviewFile[] };
  showRepoHeaders: boolean;
  hasWorkspaceRootGroup: boolean;
  renderFile: (file: ReviewFile) => ReactNode;
};

export function ReviewDiffGroup({
  group,
  showRepoHeaders,
  hasWorkspaceRootGroup,
  renderFile,
}: ReviewDiffGroupProps) {
  return (
    <div data-testid="changes-repo-group" data-repository-name={group.repositoryName}>
      {showRepoHeaders && (
        <RepoGroupHeader
          name={group.repositoryName}
          fileCount={group.items.length}
          isSubmodule={
            Boolean(group.repositoryName) &&
            (hasWorkspaceRootGroup || group.repositoryName.includes("/"))
          }
        />
      )}
      {group.items.map((file) => renderFile(file))}
    </div>
  );
}
