"use client";

import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { DesktopIntegrationChangeRequestStatus } from "./integration-change-request-status-desktop";
import { MobileIntegrationChangeRequestStatus } from "./integration-change-request-status-mobile";
import type { IntegrationChangeRequestStatusProps } from "./integration-change-request-status-types";

export type {
  IntegrationChangeRequestPipelineRow,
  IntegrationChangeRequestPipelineState,
  IntegrationChangeRequestReviewSummary,
  IntegrationChangeRequestStatusItem,
  IntegrationChangeRequestStatusProps,
} from "./integration-change-request-status-types";

export function IntegrationChangeRequestStatus({
  items,
  surface = "topbar",
}: IntegrationChangeRequestStatusProps) {
  const usesTouchDrawer = useTouchDrawer();
  if (items.length === 0) return null;
  if (usesTouchDrawer && surface === "composer") {
    return <MobileIntegrationChangeRequestStatus items={items} surface={surface} />;
  }
  return (
    <DesktopIntegrationChangeRequestStatus
      items={items}
      surface={surface}
      hoverDisabled={usesTouchDrawer}
    />
  );
}
