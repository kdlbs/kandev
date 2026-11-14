"use client";

import { useEffect } from "react";
import { TokenUsagePageClient } from "./token-usage-page-client";
import type { RangeKey } from "./stats-utils";
import { useOptionalAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";

type TokenUsageRouteProps = {
  workspaceId?: string;
  activeRange?: RangeKey;
  initialError: string | null;
};

// The retained Token Usage route is part of the usage history release toggle.
// Redirect to the overview when the feature is off so a bookmark cannot render
// an empty report as if it were authoritative.
export function TokenUsageRoute({ workspaceId, activeRange, initialError }: TokenUsageRouteProps) {
  const enabled = useOptionalAppStore((state) => state.features.usageHistory, false);
  const router = useRouter();

  useEffect(() => {
    if (!enabled) router.replace("/stats", { scroll: false });
  }, [enabled, router]);

  if (!enabled) return null;
  return (
    <TokenUsagePageClient
      workspaceId={workspaceId}
      activeRange={activeRange}
      initialError={initialError}
    />
  );
}
