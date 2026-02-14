'use client';

import { useEffect } from 'react';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';
import { getTaskPlan } from '@/lib/api/domains/plan-api';

type LazyPlanPreviewProps = {
  taskId: string | null;
};

export function LazyPlanPreview({ taskId }: LazyPlanPreviewProps) {
  const plan = useAppStore((state) =>
    taskId ? state.taskPlans.byTaskId[taskId] ?? null : null
  );
  const loaded = useAppStore((state) =>
    taskId ? state.taskPlans.loadedByTaskId[taskId] ?? false : false
  );
  const loading = useAppStore((state) =>
    taskId ? state.taskPlans.loadingByTaskId[taskId] ?? false : false
  );

  const storeApi = useAppStoreApi();

  useEffect(() => {
    if (!taskId || loaded || loading) return;

    const { setTaskPlanLoading } = storeApi.getState();
    setTaskPlanLoading(taskId, true);

    getTaskPlan(taskId).then((result) => {
      storeApi.getState().setTaskPlan(taskId, result);
    }).catch(() => {
      storeApi.getState().setTaskPlanLoading(taskId, false);
    });
  }, [taskId, loaded, loading, storeApi]);

  if (!taskId) {
    return <div className="text-xs text-muted-foreground">No task selected</div>;
  }

  if (loading || !loaded) {
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

  const preview = plan.content.length > 2000 ? plan.content.slice(0, 2000) + '...' : plan.content;

  return (
    <div className="space-y-1.5">
      <div className="text-muted-foreground text-xs font-medium">Plan</div>
      <pre className="text-[10px] leading-tight font-mono whitespace-pre-wrap break-all">
        {preview}
      </pre>
    </div>
  );
}
