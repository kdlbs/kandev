"use client";

import { openSessionFolder } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";

export function useOpenSessionFolder(sessionId?: string | null) {
  const request = useRequest(async () => {
    if (!sessionId) {
      return null;
    }
    const response = await openSessionFolder(sessionId, { cache: "no-store" });
    return response ?? null;
  });

  return {
    open: () => request.run(),
    status: request.status,
    isLoading: request.isLoading,
  };
}
