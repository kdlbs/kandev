"use client";

import { createContext, useContext } from "react";

export type QueueMergeContextValue = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
};

const QueueMergeContext = createContext<QueueMergeContextValue | null>(null);

/** Present only while merge is enabled; a Queue row renders its change controls from it. */
export const QueueMergeProvider = QueueMergeContext.Provider;

export function useQueueMerge(): QueueMergeContextValue | null {
  return useContext(QueueMergeContext);
}
