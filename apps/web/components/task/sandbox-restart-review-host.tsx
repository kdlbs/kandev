"use client";

import { useRef } from "react";
import { useStore } from "zustand";
import { Dialog, DialogContent } from "@kandev/ui/dialog";
import { useAppStoreApi } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useSandboxRestartReview } from "@/hooks/domains/session/use-sandbox-restart-review";
import {
  getSessionRestartReviewStore,
  settleSessionRestartReview,
  type SessionRestartReview,
} from "@/lib/session/session-restart-review";

import { RestartReviewHeader, RestartReviewActions } from "./sandbox-restart-review-controls";
import { RestartReviewPolicies } from "./sandbox-restart-policies";

export function SandboxRestartReviewHost() {
  const owner = useAppStoreApi();
  const request = useStore(getSessionRestartReviewStore(owner));
  return (
    <Dialog
      open={Boolean(request)}
      onOpenChange={(open) => {
        if (!open && request) settleSessionRestartReview(owner, request, null);
      }}
    >
      {request && <RestartReview request={request} />}
    </Dialog>
  );
}

function RestartReview({ request }: { request: SessionRestartReview }) {
  const { isMobile } = useResponsiveBreakpoint();
  const model = useSandboxRestartReview(request);
  const previousFocus = useRef(document.activeElement as HTMLElement | null);
  return (
    <DialogContent
      showCloseButton={false}
      enterConfirms={false}
      onCloseAutoFocus={(event) => {
        const target = previousFocus.current?.isConnected
          ? previousFocus.current
          : document.querySelector<HTMLElement>(
              `[data-testid="execution-control-${CSS.escape(request.sessionId)}"]`,
            );
        if (target) {
          event.preventDefault();
          target.focus();
        }
      }}
      className={
        isMobile
          ? "inset-0 h-dvh max-h-dvh w-full max-w-none sm:max-w-none translate-x-0 translate-y-0 rounded-none border-0 flex flex-col overflow-hidden pt-[max(1rem,env(safe-area-inset-top))] pb-[max(1rem,env(safe-area-inset-bottom))]"
          : "flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-w-xl"
      }
    >
      <RestartReviewHeader model={model} isMobile={isMobile} />
      <RestartReviewPolicies model={model} />
      <RestartReviewActions model={model} />
    </DialogContent>
  );
}
