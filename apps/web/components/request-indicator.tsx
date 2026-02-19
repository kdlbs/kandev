"use client";

import { IconCheck, IconLoader2, IconX } from "@tabler/icons-react";
import type { RequestStatus } from "@/lib/http/use-request";

type RequestIndicatorProps = {
  status: RequestStatus;
};

export function RequestIndicator({ status }: RequestIndicatorProps) {
  if (status === "loading") {
    return <IconLoader2 className="h-4 w-4 animate-spin text-muted-foreground" />;
  }
  if (status === "success") {
    return <IconCheck className="h-4 w-4 text-emerald-500" />;
  }
  if (status === "error") {
    return <IconX className="h-4 w-4 text-destructive" />;
  }
  return null;
}
