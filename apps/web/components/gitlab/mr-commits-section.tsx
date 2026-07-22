import { IconGitCommit } from "@tabler/icons-react";
import type { GitLabMRCommit } from "@/lib/types/gitlab";
import { CollapsibleSection, formatTimeAgo } from "@/components/github/pr-shared";

export function MRCommitsSection({ commits }: { commits: GitLabMRCommit[] }) {
  return (
    <CollapsibleSection title="Commits" count={commits.length} defaultOpen={false}>
      {commits.length === 0 && (
        <p className="px-2 py-2 text-xs text-muted-foreground">No commits</p>
      )}
      {commits.map((commit) => (
        <div
          key={commit.sha}
          className="flex min-w-0 items-start gap-2 rounded-md border px-2 py-2"
        >
          <IconGitCommit className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <p className="line-clamp-2 text-xs font-medium">{commit.message}</p>
            <p className="mt-1 truncate text-[10px] text-muted-foreground">
              {commit.author_name} · {formatTimeAgo(commit.author_date)}
            </p>
          </div>
          <code className="shrink-0 text-[10px] text-muted-foreground">
            {commit.sha.slice(0, 8)}
          </code>
        </div>
      ))}
    </CollapsibleSection>
  );
}
