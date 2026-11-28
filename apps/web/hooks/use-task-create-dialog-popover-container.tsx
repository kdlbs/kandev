"use client";

import { createContext, useContext, type ReactNode } from "react";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";

const TaskCreateDialogPopoverContainerContext = createContext<HTMLElement | null>(null);

export function TaskCreateDialogPopoverContainerProvider({
  container,
  children,
}: {
  container: HTMLElement | null;
  children: ReactNode;
}) {
  return (
    <TaskCreateDialogPopoverContainerContext.Provider value={container}>
      {children}
    </TaskCreateDialogPopoverContainerContext.Provider>
  );
}

export function useTaskCreateDialogPopoverContainer() {
  return useContext(TaskCreateDialogPopoverContainerContext);
}

export function useTaskCreateDialogPortalContainer() {
  const dialogPopoverContainer = useTaskCreateDialogPopoverContainer();
  const touchDrawer = useTouchDrawer();
  return touchDrawer ? null : dialogPopoverContainer;
}
