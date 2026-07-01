"use client";

import { useQuery } from "@tanstack/react-query";
import { taskPlanQueryOptions } from "@/lib/query/query-options";

type LazyPlanPreviewProps = {
  taskId: string | null;
};

export function LazyPlanPreview({ taskId }: LazyPlanPreviewProps) {
  const planQuery = useQuery(taskPlanQueryOptions(taskId ?? ""));
  const plan = planQuery.data;

  if (!taskId) {
    return <div className="text-xs text-muted-foreground">No task selected</div>;
  }

  if (planQuery.isFetching || plan === undefined) {
    return (
      <div className="space-y-1.5">
        <div className="text-muted-foreground text-xs font-medium">Plan</div>
        <div className="h-3 w-3/4 bg-muted animate-pulse rounded" />
        <div className="h-3 w-1/2 bg-muted animate-pulse rounded" />
      </div>
    );
  }

  if (!plan?.content) {
    return <div className="text-xs text-muted-foreground">Plan is empty</div>;
  }

  const preview = plan.content.length > 2000 ? plan.content.slice(0, 2000) + "..." : plan.content;

  return (
    <div className="space-y-1.5">
      <div className="text-muted-foreground text-xs font-medium">Plan</div>
      <pre className="text-[10px] leading-tight font-mono whitespace-pre-wrap break-all">
        {preview}
      </pre>
    </div>
  );
}
