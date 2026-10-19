"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { AutonomyInput } from "@/hooks/domains/coordinator/use-autonomy";
import { usePauseControl } from "@/hooks/domains/coordinator/use-pause-control";
import { useCoordinatorPhase31Effective } from "@/hooks/domains/settings/use-coordinator-phase31-effective";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";

type Props = {
  workspaceId: string;
  coordinatorId: string;
  autonomy: AutonomyInput;
  canManage: boolean;
  testIdPrefix?: string;
};

/**
 * The Pause or Resume button: only for a manager while the phase 3.1 flag is
 * effective, disabled while the request is in flight, full width with a 44 px
 * target on a phone.
 */
export function PauseControl({
  workspaceId,
  coordinatorId,
  autonomy,
  canManage,
  testIdPrefix = "autonomy",
}: Props) {
  const { t } = useTranslation();
  const flagOn = useCoordinatorPhase31Effective();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const { pending, failed, setPaused } = usePauseControl(workspaceId, coordinatorId, autonomy);
  const value = autonomy.value;
  if (!flagOn || !canManage || !value) return null;
  const paused = value.paused === true;
  const touch = isMobile || !isFinePointer;
  return (
    <span className={cn("inline-flex flex-wrap items-center gap-2", isMobile && "w-full")}>
      <Button
        variant="outline"
        size="sm"
        disabled={pending}
        onClick={() => setPaused(!paused)}
        className={cn("cursor-pointer", touch && "min-h-11 min-w-11", isMobile && "w-full")}
        data-testid={`${testIdPrefix}-${paused ? "resume" : "pause"}`}
      >
        {paused ? t("coordinator:autonomyResume") : t("coordinator:autonomyPause")}
      </Button>
      {failed && (
        <span
          role="alert"
          className="text-destructive"
          data-testid={`${testIdPrefix}-pause-failed`}
        >
          {t("coordinator:autonomyPauseFailed")}
        </span>
      )}
    </span>
  );
}

/** The note beside a read-only Paused badge while the phase 3.1 flag is off. */
export function PausedFlagOffNote({ paused }: { paused: boolean }) {
  const { t } = useTranslation();
  const flagOn = useCoordinatorPhase31Effective();
  if (!paused || flagOn) return null;
  return (
    <span className="text-muted-foreground text-xs" data-testid="autonomy-paused-flag-off-note">
      {t("coordinator:autonomyPausedFlagOff")}
    </span>
  );
}
