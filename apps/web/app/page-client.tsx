"use client";

import { useEffect, useState } from "react";

import { KanbanWithPreview } from "@/components/kanban-with-preview";
import { OnboardingDialog } from "@/components/onboarding-dialog";
import { useAppStore } from "@/components/state-provider";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import { linkToTask, linkToTasks, linkToThreads } from "@/lib/links";
import { getRecentTasks } from "@/lib/recent-tasks";
import { isExplicitHomeDestination, resolveStartupTaskId } from "@/lib/startup-page";
import { resolveHomeTaskListingRedirect } from "@/lib/task-listing/view-preference";
import { useTranslation } from "react-i18next";

type PageClientProps = {
  workspaceId?: string;
  initialTaskId?: string;
  initialSessionId?: string;
};

export function PageClient({ workspaceId, initialTaskId, initialSessionId }: PageClientProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { preferredView } = useTaskListingView();
  const startupPage = useAppStore((state) => state.userSettings.startupPage);
  const hasExplicitDestination = isExplicitHomeDestination(
    searchParams,
    initialTaskId,
    initialSessionId,
  );
  const [recentTasks, setRecentTasks] = useState<ReturnType<typeof getRecentTasks> | null>(null);
  const startupTaskId = resolveStartupTaskId({
    startupPage,
    workspaceId,
    recentTasks: recentTasks ?? [],
    hasExplicitDestination,
  });
  const isResolvingStartupTask =
    startupPage === "last_task" && !hasExplicitDestination && recentTasks === null;
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
    setRecentTasks(getRecentTasks());
  }, []);

  useEffect(() => {
    if (isResolvingStartupTask) return;
    if (startupTaskId) {
      router.replace(linkToTask(startupTaskId));
      return;
    }
    if (hasWorkflowFilter) return;
    const routedView = resolveHomeTaskListingRedirect(
      preferredView,
      initialTaskId,
      initialSessionId,
    );
    if (routedView === "list") router.replace(linkToTasks(workspaceId));
    if (routedView === "threads") router.replace(linkToThreads(workspaceId));
  }, [
    hasWorkflowFilter,
    initialSessionId,
    initialTaskId,
    isResolvingStartupTask,
    preferredView,
    router,
    startupTaskId,
    workspaceId,
  ]);

  if (isResolvingStartupTask || startupTaskId) {
    return (
      <div className="flex h-full min-h-0 w-full items-center justify-center bg-background">
        <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
          {t("common:openingLastTask")}
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
