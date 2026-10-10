"use client";

import { createContext, useContext, type ReactNode } from "react";

const TaskRouteSessionHydrationContext = createContext(true);

export function TaskRouteSessionHydrationProvider({
  isReady,
  children,
}: {
  isReady: boolean;
  children: ReactNode;
}) {
  return (
    <TaskRouteSessionHydrationContext.Provider value={isReady}>
      {children}
    </TaskRouteSessionHydrationContext.Provider>
  );
}

export function useTaskRouteSessionHydrated(): boolean {
  return useContext(TaskRouteSessionHydrationContext);
}
