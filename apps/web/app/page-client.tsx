"use client";

import { useEffect, useState } from "react";

import { KanbanWithPreview } from "@/components/kanban-with-preview";
import { OnboardingDialog } from "@/components/onboarding-dialog";
import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { useRouter } from "@/lib/routing/client-router";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import { linkToTasks } from "@/lib/links";
import { shouldRestoreHomeTaskListingView } from "@/lib/task-listing/view-preference";

type PageClientProps = {
  workspaceId?: string;
  initialTaskId?: string;
  initialSessionId?: string;
};

export function PageClient({ workspaceId, initialTaskId, initialSessionId }: PageClientProps) {
  const router = useRouter();
  const { preferredView } = useTaskListingView();
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
    if (shouldRestoreHomeTaskListingView(preferredView, initialTaskId, initialSessionId)) {
      router.replace(linkToTasks(workspaceId));
    }
  }, [initialSessionId, initialTaskId, preferredView, router, workspaceId]);

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
