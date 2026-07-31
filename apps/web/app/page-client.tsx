"use client";

import { useEffect, useState } from "react";

import { KanbanWithPreview } from "@/components/kanban-with-preview";
import { OnboardingDialog } from "@/components/onboarding-dialog";
import { useAppStore } from "@/components/state-provider";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import { linkToTask, linkToTasks } from "@/lib/links";
import { getRecentTasks } from "@/lib/recent-tasks";
import { isExplicitHomeDestination, resolveStartupTaskId } from "@/lib/startup-page";
import { shouldRestoreHomeTaskListingView } from "@/lib/task-listing/view-preference";

type PageClientProps = {
  workspaceId?: string;
  initialTaskId?: string;
  initialSessionId?: string;
};

export function PageClient({ workspaceId, initialTaskId, initialSessionId }: PageClientProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const { preferredView } = useTaskListingView();
  const startupPage = useAppStore((state) => state.userSettings.startupPage);
  const hasExplicitDestination = isExplicitHomeDestination(
    searchParams,
    initialTaskId,
    initialSessionId,
  );
  const startupTaskId = resolveStartupTaskId({
    startupPage,
    workspaceId,
    recentTasks: getRecentTasks(),
    hasExplicitDestination,
  });
  const hasWorkflowFilter = Boolean(searchParams.get("workflowId"));
  const [showOnboarding, setShowOnboarding] = useState(() => {
    if (typeof window === "undefined") return false;
    const completed = getLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, false);
    return !completed;
  });
  const [boardKey, setBoardKey] = useState(0);

  const handleOnboardingComplete = () => {
    setLocalStorage(STORAGE_KEYS.ONBOARDING_COMPLETED, true);
    setShowOnboarding(false);
    setBoardKey((prev) => prev + 1);
  };

  useEffect(() => {
    if (startupTaskId) {
      router.replace(linkToTask(startupTaskId));
      return;
    }
    if (hasWorkflowFilter) return;
    if (shouldRestoreHomeTaskListingView(preferredView, initialTaskId, initialSessionId)) {
      router.replace(linkToTasks(workspaceId));
    }
  }, [
    hasWorkflowFilter,
    initialSessionId,
    initialTaskId,
    preferredView,
    router,
    startupTaskId,
    workspaceId,
  ]);

  if (startupTaskId) {
    return (
      <div className="flex h-full min-h-0 w-full items-center justify-center bg-background">
        <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
          Opening last task…
        </p>
      </div>
    );
  }

  return (
    <>
      <OnboardingDialog open={showOnboarding} onComplete={handleOnboardingComplete} />
      <KanbanWithPreview
        key={boardKey}
        initialTaskId={initialTaskId}
        initialSessionId={initialSessionId}
      />
    </>
  );
}
