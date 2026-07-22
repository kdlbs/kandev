import {
  IconAlertTriangle,
  IconCheck,
  IconGitBranch,
  IconGitMerge,
  IconX,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import type { GitLabMRFeedback, TaskMR } from "@/lib/types/gitlab";
import { CollapsibleSection, PRMarkdownBody } from "@/components/github/pr-shared";

function pipelineTone(status: string): string {
  if (status === "success") return "text-green-600";
  if (["failed", "canceled"].includes(status)) return "text-red-600";
  return "text-yellow-600";
}

export function pipelineSummary(
  pipeline: GitLabMRFeedback["pipelines"][number] | undefined,
): string {
  if (!pipeline) return "No pipeline";
  if (pipeline.jobs_total <= 0) return pipeline.status;
  return `${pipeline.status} · ${pipeline.jobs_passing}/${pipeline.jobs_total}`;
}

export function MROverviewSection({
  taskMR,
  feedback,
}: {
  taskMR: TaskMR;
  feedback: GitLabMRFeedback;
}) {
  const mr = feedback.mr;
  const pipeline = feedback.pipelines[0];
  const mergeable = !mr.has_conflicts && ["can_be_merged", "mergeable"].includes(mr.merge_status);
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge variant="outline" className="capitalize">
          {mr.state}
        </Badge>
        {mr.draft && <Badge variant="secondary">Draft</Badge>}
        <span className="flex min-w-0 items-center gap-1">
          <IconGitBranch className="h-3.5 w-3.5 shrink-0" />
          <code className="max-w-44 truncate">{mr.head_branch}</code>
          <span className="text-muted-foreground">into</span>
          <code className="max-w-44 truncate">{mr.base_branch}</code>
        </span>
      </div>
      <div className="grid gap-2 sm:grid-cols-3">
        <div className="rounded-md border p-2 text-xs">
          <span className="text-muted-foreground">Mergeability</span>
          <p className="mt-1 flex items-center gap-1 font-medium">
            {mergeable ? (
              <IconCheck className="h-4 w-4 text-green-600" />
            ) : (
              <IconAlertTriangle className="h-4 w-4 text-yellow-600" />
            )}
            {mr.has_conflicts ? "Conflicts" : mr.merge_status.replaceAll("_", " ")}
          </p>
        </div>
        <div className="rounded-md border p-2 text-xs">
          <span className="text-muted-foreground">Approvals</span>
          <p className="mt-1 flex items-center gap-1 font-medium">
            {feedback.approvals.length > 0 ? (
              <IconCheck className="h-4 w-4 text-green-600" />
            ) : (
              <IconX className="h-4 w-4 text-muted-foreground" />
            )}
            {feedback.approvals.length} approved
          </p>
        </div>
        <div className="rounded-md border p-2 text-xs">
          <span className="text-muted-foreground">Pipeline</span>
          <p
            className={`mt-1 flex items-center gap-1 font-medium ${pipelineTone(pipeline?.status ?? "pending")}`}
          >
            <IconGitMerge className="h-4 w-4" />
            {pipelineSummary(pipeline)}
          </p>
        </div>
      </div>
      {feedback.approvals.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {feedback.approvals.map((approval) => (
            <Badge key={`${approval.username}:${approval.created_at}`} variant="secondary">
              {approval.username}
            </Badge>
          ))}
        </div>
      )}
      {mr.body && (
        <CollapsibleSection title="Description" count={1} defaultOpen={false}>
          <div className="px-2">
            <PRMarkdownBody body={mr.body} />
          </div>
        </CollapsibleSection>
      )}
      <p className="sr-only">
        Linked merge request {taskMR.project_path}!{taskMR.mr_iid}
      </p>
    </div>
  );
}
