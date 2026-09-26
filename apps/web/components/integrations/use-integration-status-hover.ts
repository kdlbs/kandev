"use client";

import { useEffect, useRef } from "react";
import type { IntegrationChangeRequestStatusItem } from "./integration-change-request-status-types";
import { useHoverPopover } from "./use-hover-popover";

const HOVER_DELAY_MS = 150;

export function useIntegrationStatusHover(disabled: boolean) {
  return useHoverPopover({
    openDelayMs: HOVER_DELAY_MS,
    closeDelayMs: HOVER_DELAY_MS,
    disabled,
  });
}

export function useRefreshItemsWhenOpen(
  open: boolean,
  items: readonly IntegrationChangeRequestStatusItem[],
) {
  const wasOpen = useRef(false);
  useEffect(() => {
    if (open && !wasOpen.current) {
      for (const item of items) void item.onRefresh?.();
    }
    wasOpen.current = open;
  }, [items, open]);
}
