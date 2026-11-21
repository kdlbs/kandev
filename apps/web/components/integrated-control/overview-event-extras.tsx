"use client";

import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  OverviewEvent,
  OverviewFailure,
  OverviewStepMove,
} from "@/lib/types/control-overview";
import {
  blockFacts,
  durationFromMinutes,
  modelBlockSubject,
  occurredTime,
  sessionStateLabel,
  taskStateLabel,
} from "./overview-format";
import { statusTextClass } from "./overview-status-colors";

// The second line under an event row: what a run of step moves went through, or
// what a failure was followed by. Both are subordinate to the row itself, so
// they read as smaller supporting text rather than as another event.

/**
 * A task's run of step changes as one summary and a list behind it.
 *
 * The summary answers what a reader asks first: how often the task moved, how
 * often work was sent back, how often finished work was opened again, whether it
 * is parked, and where it stands now. The path itself is a disclosure rather
 * than a line of its own, because a run of dozens of steps on one row reads as
 * noise and buries the two movements worth acting on.
 */
function StepMoveLine({ event }: { event: OverviewEvent }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const listId = useId();
  const moves = event.moves ?? [];
  if (moves.length === 0) return null;
  const current = moves[moves.length - 1];
  const total = event.move_total ?? moves.length;
  const span =
    event.from && event.to && event.from !== event.to
      ? `${occurredTime(event.from)}-${occurredTime(event.to)}`
      : occurredTime(event.to ?? event.from);
  const figures: SummaryFigure[] = [
    { label: t("office:overviewStepMoveTotal", { count: total }), testId: "total" },
  ];
  if (event.sent_back) {
    figures.push({
      label: t("office:overviewStepMoveSentBack", { count: event.sent_back }),
      tone: "error",
      testId: "sent-back",
    });
  }
  if (event.reopened) {
    figures.push({
      label: t("office:overviewStepMoveReopened", { count: event.reopened }),
      tone: "hold",
      testId: "reopened",
    });
  }
  if (event.held) {
    figures.push({ label: t("office:overviewStepMoveHeld"), tone: "hold", testId: "held" });
  }
  return (
    <span className="mt-0.5 block text-xs text-muted-foreground">
      <span
        className="flex flex-wrap items-center gap-x-1.5"
        data-testid="overview-step-move-summary"
      >
        <span className="font-medium text-foreground">{stepName(t, current)}</span>
        {figures.map((figure, index) => (
          <span key={figure.testId} className="flex items-center gap-x-1.5">
            {index > 0 && <span aria-hidden="true">{" · "}</span>}
            <span
              className={figure.tone ? statusTextClass(figure.tone) : undefined}
              data-testid={`overview-step-move-${figure.testId}`}
            >
              {figure.label}
            </span>
          </span>
        ))}
        {span && (
          <span className="flex items-center gap-x-1.5">
            <span aria-hidden="true">{" · "}</span>
            <span className="tabular-nums">{span}</span>
          </span>
        )}
        <StepMoveToggle open={open} listId={listId} onToggle={() => setOpen((value) => !value)} />
      </span>
      {open && (
        <span id={listId} className="mt-1 block" data-testid="overview-step-move-path">
          {moves.map((move, index) => (
            <StepMoveRow key={`${move.at}-${index}`} move={move} />
          ))}
        </span>
      )}
    </span>
  );
}

/**
 * One figure of the summary. A tone is applied only to a figure that reports
 * something to act on, so the two movements worth noticing stand out and the
 * rest of the line stays quiet.
 */
type SummaryFigure = {
  label: string;
  tone?: "error" | "hold";
  testId: string;
};

/**
 * The disclosure control. The event row is a link to the task, so this is a
 * focusable span rather than a button: a button nested in a link is neither, and
 * activating it would navigate away from the row being read. Enter and Space
 * behave as they do on a button, and the click is kept from reaching the link.
 * Its hit area is the phone minimum and the desktop text density, like the
 * sibling disclosure on the workspace card, because opening the chain is one of
 * the ways a reader uses this screen.
 */
function StepMoveToggle({
  open,
  listId,
  onToggle,
}: {
  open: boolean;
  listId: string;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  return (
    <span
      role="button"
      tabIndex={0}
      aria-expanded={open}
      aria-controls={listId}
      className="inline-flex min-h-11 cursor-pointer items-center text-xs text-muted-foreground hover:underline sm:min-h-0"
      data-testid="overview-step-move-toggle"
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onToggle();
      }}
      onKeyDown={(event) => {
        if (event.key !== "Enter" && event.key !== " ") return;
        event.preventDefault();
        event.stopPropagation();
        onToggle();
      }}
    >
      {open ? (
        <IconChevronUp className="mr-1 inline h-3 w-3" aria-hidden="true" />
      ) : (
        <IconChevronDown className="mr-1 inline h-3 w-3" aria-hidden="true" />
      )}
      {open ? t("office:overviewCollapseDetails") : t("office:overviewExpandDetails")}
    </span>
  );
}

/**
 * One movement: when it happened, the step it left and the step it arrived at,
 * and who moved it. The two step names are kept apart by the arrow rather than
 * set next to each other, because a step name beside its actor reads as one
 * phrase. A movement the fold absorbed repeats as a count.
 */
function StepMoveRow({ move }: { move: OverviewStepMove }) {
  const { t } = useTranslation();
  return (
    <span className="mt-0.5 flex flex-wrap items-center gap-x-1.5">
      <span className="tabular-nums">{occurredTime(move.at)}</span>
      {move.from_step_name && <span>{move.from_step_name}</span>}
      {move.from_step_name && <span aria-hidden="true">{" → "}</span>}
      <span className={move.stopped ? statusTextClass("hold") : undefined}>
        {stepName(t, move)}
      </span>
      {move.stopped && (
        <span className="text-muted-foreground/80" data-testid="overview-step-move-stopped">
          {t("office:overviewEventStepStopped")}
        </span>
      )}
      {move.reopened && (
        <span className={statusTextClass("hold")} data-testid="overview-step-move-row-reopened">
          {t("office:overviewStepMoveReopenedOne")}
        </span>
      )}
      {move.sent_back && (
        <span className={statusTextClass("error")} data-testid="overview-step-move-row-sent-back">
          {t("office:overviewStepMoveSentBackOne")}
        </span>
      )}
      {move.actor && (
        <span className="text-muted-foreground/80">
          {t(STEP_ACTOR_KEYS[move.actor] ?? "office:overviewEventActorUnknown")}
        </span>
      )}
      {move.repeat && move.repeat > 1 && (
        <span className="tabular-nums" data-testid="overview-step-move-repeat">
          {t("office:overviewStepMoveRepeat", { times: move.repeat })}
        </span>
      )}
    </span>
  );
}

/** A step's name, or the screen's wording for a step the ledger cannot name. */
function stepName(t: (key: string) => string, move: OverviewStepMove): string {
  return move.step_name || t("office:overviewStepUnnamed");
}

// The ledger's actor kinds, phrased as who moved a task. An actor the screen has
// no wording for is reported as unknown rather than guessed at.
const STEP_ACTOR_KEYS: Record<string, string> = {
  human: "office:overviewEventActorHuman",
  agent: "office:overviewEventActorAgent",
  system: "office:overviewEventActorSystem",
  integration: "office:overviewEventActorIntegration",
};

/**
 * What a failed session was followed by: the session that took over, whether
 * routing moved to another candidate, and where the task ended up.
 *
 * The three are reported independently because they are independent: a task can
 * have been retried by an agent and still be sitting on hold. When none of them
 * found anything, that is stated outright rather than left as an uneventful row,
 * because silence here is the finding, not an absence of one. An execution actor
 * is shown only when the records name one.
 */
export function FailureLine({ failure }: { failure: OverviewFailure }) {
  const { t } = useTranslation();
  const parts: string[] = [];
  const next = failure.next_session;
  if (next) {
    const model = next.model_name || t("office:overviewStepUnnamed");
    parts.push(
      t("office:overviewFailureNextSession", {
        model,
        state: sessionStateLabel(t, next.state),
        when: occurredTime(next.started_at),
      }),
    );
  }
  if (failure.route_attempts && failure.route_attempts > 0) {
    // A reason the routing catalog names is stated in the active locale; one it
    // does not name stays as the code, which is the diagnostic it was.
    parts.push(
      t("office:overviewFailureRouted", {
        count: failure.route_attempts,
        reason: failure.route_reason || t("office:overviewFailureRoutedNoReason"),
      }),
    );
  }
  if (failure.task_state) {
    parts.push(
      t("office:overviewFailureTaskState", { state: taskStateLabel(t, failure.task_state) }),
    );
  }
  if (failure.has_no_action) {
    return (
      <span
        className={`mt-0.5 block text-xs font-medium ${statusTextClass("error")}`}
        data-testid="overview-failure-no-action"
      >
        {t("office:overviewFailureNoAction", {
          duration: durationFromMinutes(failure.failed_ago_minutes),
        })}
      </span>
    );
  }
  if (parts.length === 0) return null;
  return (
    <span
      className="mt-0.5 block truncate text-xs text-muted-foreground"
      data-testid="overview-failure-followup"
    >
      {parts.join(" · ")}
    </span>
  );
}

/**
 * The supporting line an event row carries, or nothing when it carries none.
 *
 * The server sends these facts as fields rather than prose so the client can
 * phrase them, which means every field it fills has to be read here: a value
 * that arrives and is never rendered is a requirement that quietly does not
 * reach the screen.
 */
export function OverviewEventExtras({ event }: { event: OverviewEvent }) {
  const { t } = useTranslation();
  const block = modelBlockSubject(event) ? blockFacts(t, event.scope, event.reason) : "";
  return (
    <>
      {event.version && <EventField>{event.version}</EventField>}
      {block && <EventField>{block}</EventField>}
      {event.clears_at && (
        <EventField>
          {t("office:overviewClearsAt", { time: occurredTime(event.clears_at) })}
        </EventField>
      )}
      {event.pull_request && (
        <EventField>
          {event.pull_request.owner}/{event.pull_request.repo}#{event.pull_request.number}
        </EventField>
      )}
      {event.decided_at && (
        <EventField>
          {t("office:overviewDecisionAnswered", { time: occurredTime(event.decided_at) })}
        </EventField>
      )}
      {event.moves && event.moves.length > 0 && <StepMoveLine event={event} />}
      {event.failure && <FailureLine failure={event.failure} />}
    </>
  );
}

/** One supporting fact on its own line under the row's title. */
function EventField({ children }: { children: React.ReactNode }) {
  return <span className="mt-0.5 block truncate text-xs text-muted-foreground">{children}</span>;
}
