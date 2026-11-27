import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { AutonomySpend } from "@/components/coordinators/autonomy/autonomy-spend";
import { PauseControl, PausedFlagOffNote } from "@/components/coordinators/autonomy/pause-control";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AutonomyInput } from "@/hooks/domains/coordinator/use-autonomy";
import { isStopFailing } from "@/lib/coordinator/autonomy";
import { heldReasonText } from "@/lib/coordinator/autonomy-text";
import {
  lastWokeAgeMs,
  stripState,
  stripVisible,
  type StripState,
} from "@/lib/coordinator/autonomy-strip";
import { formatAge } from "@/lib/coordinator/format";
import { stopSessionTurn } from "@/lib/coordinator/stop-turn";
import { formatTime } from "@/lib/i18n/formats";
import { cn } from "@/lib/utils";
import { useNowTick } from "../use-now-tick";

type TFn = ReturnType<typeof useTranslation>["t"];

function pausedText(state: Extract<StripState, { kind: "paused" }>, t: TFn): string {
  const time = state.at ? formatTime(Date.parse(state.at)) : null;
  if (state.by && time) return t("coordinator:autonomyStatePausedByAt", { name: state.by, time });
  if (state.by) return t("coordinator:autonomyStatePausedBy", { name: state.by });
  if (time) return t("coordinator:autonomyStatePausedAt", { time });
  return t("coordinator:autonomyStatePaused");
}

function stateText(state: StripState, t: TFn): { text: string; tone: string } {
  switch (state.kind) {
    case "off":
      return { text: t("coordinator:autonomyStateOff"), tone: "off" };
    case "paused":
      return { text: pausedText(state, t), tone: "paused" };
    case "active":
      return { text: t("coordinator:autonomyStateActive"), tone: "active" };
    case "busy":
      return {
        text: t("coordinator:autonomyStateActiveWith", {
          detail: t("coordinator:autonomyTransientBusy"),
        }),
        tone: "active",
      };
    case "cooldown":
      return {
        text: t("coordinator:autonomyStateActiveWith", {
          detail: t("coordinator:autonomyTransientCooldown", { time: formatTime(state.untilMs) }),
        }),
        tone: "active",
      };
    case "held":
      return {
        text: t("coordinator:autonomyStateHeld", {
          reason: heldReasonText(state.reason, state.detail, t),
        }),
        tone: "held",
      };
    case "unknown":
      return {
        text: t("coordinator:autonomyStateHeld", { reason: t("coordinator:autonomyHeldUnknown") }),
        tone: "unknown",
      };
  }
}

function useStop(sessionId: string) {
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  const stop = useCallback(() => {
    if (pending) return;
    setPending(true);
    stopSessionTurn(sessionId).then(
      () => {
        setFailed(false);
        setPending(false);
      },
      () => {
        setFailed(true);
        setPending(false);
      },
    );
  }, [pending, sessionId]);
  return { pending, failed, stop };
}

function StopWarning({ sessionId, canManage }: { sessionId: string; canManage: boolean }) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const { pending, failed, stop } = useStop(sessionId);
  return (
    <div
      className="flex flex-wrap items-center gap-2 text-sm text-destructive"
      data-testid="autonomy-stop-warning"
    >
      <span>{t("coordinator:autonomyStopWarning")}</span>
      {canManage && (
        <Button
          variant="outline"
          size="sm"
          disabled={pending}
          onClick={stop}
          className={cn("cursor-pointer", (isMobile || !isFinePointer) && "min-h-11 min-w-11")}
          data-testid="autonomy-stop"
        >
          {t("coordinator:autonomyStop")}
        </Button>
      )}
      {canManage && failed && (
        <span role="alert" data-testid="autonomy-stop-failed">
          {t("coordinator:autonomyStopFailed")}
        </span>
      )}
    </div>
  );
}

function StripUnavailable({ retry }: { retry: () => void }) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  return (
    <div
      className="bg-background flex flex-wrap items-center gap-2 border-b px-4 py-2 text-sm"
      role="alert"
      data-testid="autonomy-strip"
      data-strip="unavailable"
    >
      <span data-testid="autonomy-strip-unavailable">{t("coordinator:autonomyUnavailable")}</span>
      <Button
        variant="outline"
        size="sm"
        onClick={retry}
        className={cn("cursor-pointer", (isMobile || !isFinePointer) && "min-h-11 min-w-11")}
        data-testid="autonomy-strip-retry"
      >
        {t("coordinator:tryAgain")}
      </Button>
    </div>
  );
}

export type AutonomyStripProps = {
  autonomy: AutonomyInput;
  canManage: boolean;
  workspaceId?: string;
  coordinatorId?: string;
};

/**
 * The autonomy strip above the count strip: state, last wake, pending count
 * and 24-hour spend, plus a warning row while a ceiling stop is not
 * confirmed. Not rendered before the first read settles; an error replaces
 * it with one retry line, whatever stale value is held.
 */
export function AutonomyStrip({
  autonomy,
  canManage,
  workspaceId,
  coordinatorId,
}: AutonomyStripProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const now = useNowTick();
  const { value, loadedAt, error } = autonomy;
  if (error) return <StripUnavailable retry={autonomy.retry} />;
  if (!value || loadedAt === null || !stripVisible(value)) return null;

  const state = stripState(value);
  const { text, tone } = stateText(state, t);
  const showDetails = state.kind !== "off";
  const ageMs = lastWokeAgeMs(value, loadedAt, now);
  const stopTurn = isStopFailing(value) ? value.last_turn : null;
  const control =
    workspaceId && coordinatorId ? (
      <PauseControl
        workspaceId={workspaceId}
        coordinatorId={coordinatorId}
        autonomy={autonomy}
        canManage={canManage}
      />
    ) : null;
  return (
    <div
      className="bg-background space-y-1 border-b px-4 py-2 text-sm"
      data-testid="autonomy-strip"
      data-strip="ready"
    >
      <div className="[&>*]:mr-2 [&>*]:align-middle" data-testid="autonomy-strip-line">
        <span className="font-medium" data-testid="autonomy-strip-state" data-state={tone}>
          {text}
        </span>{" "}
        {showDetails && (
          <>
            {value.last_woke_at === null ? (
              <span
                className="text-muted-foreground text-xs whitespace-nowrap"
                data-testid="autonomy-strip-last-woke"
              >
                {t("coordinator:autonomyNotWoken")}
              </span>
            ) : (
              ageMs !== null && (
                <span
                  className="text-muted-foreground text-xs whitespace-nowrap"
                  data-testid="autonomy-strip-last-woke"
                >
                  {t("coordinator:autonomyLastWoke", { age: formatAge(ageMs) })}
                </span>
              )
            )}{" "}
            <span
              className="text-muted-foreground text-xs whitespace-nowrap"
              data-testid="autonomy-strip-pending"
            >
              {t("coordinator:autonomyPending", { count: value.pending_wakes })}
            </span>{" "}
            <AutonomySpend spend={value.spend} />
          </>
        )}
        <PausedFlagOffNote paused={state.kind === "paused"} />
        {!isMobile && control}
      </div>
      {isMobile && control && <div data-testid="autonomy-strip-control-row">{control}</div>}
      {stopTurn && <StopWarning sessionId={stopTurn.session_id} canManage={canManage} />}
    </div>
  );
}
