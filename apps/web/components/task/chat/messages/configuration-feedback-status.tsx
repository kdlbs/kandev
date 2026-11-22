"use client";

import { Fragment, type ReactNode } from "react";
import { IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Trans, useTranslation } from "react-i18next";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { sessionConfigurationExecutionId } from "@/components/task/session-configuration-request";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  configurationFeedbackTurnId,
  type ConfigurationFeedbackEntry,
  type SessionConfigurationFeedbackRow,
} from "@/lib/state/slices/session-runtime/session-configuration-feedback";
import type { AppState } from "@/lib/state/store";
import type { Message } from "@/lib/types/http";
import { cn } from "@/lib/utils";

export function MutedConfigurationStatus({
  message,
  testId,
  dismiss,
  retry,
}: {
  message: ReactNode;
  testId: string;
  dismiss?: () => void;
  retry?: () => void;
}) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touch = isMobile || !isFinePointer;
  return (
    <div className="flex min-w-0 flex-wrap items-start gap-x-1">
      <div
        className={cn(
          "min-w-0 break-words py-0.5 text-xs leading-relaxed text-muted-foreground",
          isMobile && (retry || dismiss) ? "w-full" : "flex-1",
        )}
        role="status"
        aria-live="polite"
        data-testid={testId}
      >
        {message}
      </div>
      {retry && (
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={cn("h-7 min-w-7 px-2 text-muted-foreground", touch && "h-11 min-w-11")}
          data-testid={`${testId}-retry`}
          onClick={retry}
        >
          {t("task:sessionConfigurationRetry")}
        </Button>
      )}
      {dismiss && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={cn("h-7 w-7 text-muted-foreground", touch && "h-11 w-11")}
          aria-label={t("common:close")}
          data-testid={`${testId}-dismiss`}
          onClick={dismiss}
        >
          <IconX className="h-3.5 w-3.5" aria-hidden />
        </Button>
      )}
    </div>
  );
}

function SettingValue({ label, value }: { label: string; value: string }) {
  return (
    <>
      {label} <code className="font-mono">{value}</code>
    </>
  );
}

function SettingsList({
  entries,
}: {
  entries: { configId: string; label: string; value: string }[];
}) {
  return (
    <span>
      {entries.map((entry, index) => (
        <Fragment key={entry.configId}>
          {index > 0 && ", "}
          <SettingValue label={entry.label} value={entry.value} />
        </Fragment>
      ))}
    </span>
  );
}

function PickerFeedbackText({ entries }: { entries: ConfigurationFeedbackEntry[] }) {
  const changed = entries.filter((entry) => entry.kind === "changed");
  const saved = entries.filter((entry) => entry.kind === "saved");
  const notices = entries.filter(
    (entry) => entry.kind === "notice" || entry.kind === "unconfirmed",
  );
  return (
    <>
      {changed.length > 0 && (
        <Trans
          i18nKey="task:sessionConfigurationChanged"
          components={{ settings: <SettingsList entries={changed} /> }}
        />
      )}
      {saved.length > 0 && (
        <>
          {changed.length > 0 && " "}
          <Trans
            i18nKey="task:sessionConfigurationSaved"
            components={[<SettingsList key="settings" entries={saved} />]}
          />
        </>
      )}
      {notices.map((entry, index) => (
        <Fragment key={entry.configId}>
          {changed.length + saved.length + index > 0 && " "}
          {entry.kind === "unconfirmed" ? (
            <span className="text-destructive">
              <Trans
                i18nKey="task:sessionConfigurationUnconfirmed"
                components={{ setting: <SettingValue label={entry.label} value={entry.value} /> }}
              />
              {entry.reason && ` ${entry.reason}`}
            </span>
          ) : (
            entry.text
          )}
        </Fragment>
      ))}
    </>
  );
}

function eligibleRetry(
  state: AppState,
  sessionId: string,
  row: SessionConfigurationFeedbackRow | undefined,
) {
  const retry = row?.retry;
  if (
    !row ||
    !retry?.executionId ||
    retry.executionId !== sessionConfigurationExecutionId(state, sessionId) ||
    row.turnId !== configurationFeedbackTurnId(state.turns.bySession[sessionId])
  ) {
    return undefined;
  }
  return retry.run;
}

export function PickerConfigurationStatus({ comment }: { comment: Message }) {
  const { session_id: sessionId, id } = comment;
  const store = useAppStoreApi();
  const row = useAppStore((state) =>
    state.configurationFeedback.bySessionId[sessionId]?.find((item) => item.id === id),
  );
  const retry = useAppStore((state) => eligibleRetry(state, sessionId, row));
  if (!row) return null;
  const dismiss = row.entries.some(
    (entry) =>
      entry.kind === "saved" ||
      entry.kind === "unconfirmed" ||
      (entry.kind === "notice" && entry.dismissible),
  )
    ? () => store.getState().dismissSessionConfigurationFeedback(sessionId, id)
    : undefined;
  return (
    <MutedConfigurationStatus
      message={<PickerFeedbackText entries={row.entries} />}
      testId="session-configuration-feedback"
      dismiss={dismiss}
      retry={
        retry
          ? () => {
              const state = store.getState();
              const current = state.configurationFeedback.bySessionId[sessionId]?.find(
                (item) => item.id === id,
              );
              eligibleRetry(state, sessionId, current)?.();
            }
          : undefined
      }
    />
  );
}
