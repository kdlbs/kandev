"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useTaskListingView } from "@/hooks/use-task-listing-view";
import { MobileTasksPageClient } from "./mobile-tasks-page-client";
import { TasksPageDesktopClient, type TasksPageClientProps } from "./tasks-page-desktop-client";

export type { TasksPageClientProps } from "./tasks-page-desktop-client";

export function TasksPageClient(props: TasksPageClientProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const { setView } = useTaskListingView();
  const setMobileSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);

  useEffect(() => {
    setView("list");
    return () => setMobileSearchOpen(false);
  }, [setMobileSearchOpen, setView]);

  return (
    <>
      <MobileTasksPageClient active={isMobile} />
      {!isMobile && <TasksPageDesktopClient {...props} />}
    </>
  );
}
