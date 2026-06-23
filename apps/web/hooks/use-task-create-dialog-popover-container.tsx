"use client";

import { createContext, useContext, type ReactNode } from "react";

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
