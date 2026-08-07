"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Spinner } from "@kandev/ui/spinner";
import { IconExternalLink } from "@tabler/icons-react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { linkToTask } from "@/lib/links";
import type {
  DiagnosticBundleCapabilities,
  DiagnosticBundleSource,
  DiagnosticSession,
} from "@/lib/types/system";

type BundleCustomizerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  capabilities: DiagnosticBundleCapabilities | null;
  sessions: DiagnosticSession[];
  sessionsLoading: boolean;
  selectedSources: DiagnosticBundleSource[];
  selectedSessionIDs: string[];
  submitting: boolean;
  onSourcesChange: (sources: DiagnosticBundleSource[]) => void;
  onSessionChange: (sessionID: string, checked: boolean) => void;
  onSessionSelectionChange: (sessionIDs: string[]) => void;
  onSubmit: () => void;
};

const SOURCE_ORDER: DiagnosticBundleSource[] = ["backend", "frontend", "runtime", "acp"];
const SOURCE_COPY: Record<DiagnosticBundleSource, { label: string; description: string }> = {
  backend: {
    label: "settings:diagnosticSourceBackend",
    description: "settings:diagnosticSourceBackendDescription",
  },
  frontend: {
    label: "settings:diagnosticSourceFrontend",
    description: "settings:diagnosticSourceFrontendDescription",
  },
  runtime: {
    label: "settings:diagnosticSourceRuntime",
    description: "settings:diagnosticSourceRuntimeDescription",
  },
  acp: {
    label: "settings:diagnosticSourceACP",
    description: "settings:diagnosticSourceACPDescription",
  },
};

export function BundleCustomizer(props: BundleCustomizerProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const sourceOptions = useMemo(() => {
    const available = new Set<DiagnosticBundleSource>(
      props.capabilities?.sources ?? ["backend", "frontend", "runtime"],
    );
    return SOURCE_ORDER.filter(
      (source) =>
        available.has(source) &&
        (source !== "acp" || props.capabilities?.acp_debug_enabled === true),
    );
  }, [props.capabilities?.acp_debug_enabled, props.capabilities?.sources]);
  const acpSelected = props.selectedSources.includes("acp");
  const selectableSessions = props.sessions.filter(
    (session) => session.acp_availability !== "unavailable",
  );
  const validationKey = validationMessageKey(
    props.selectedSources,
    acpSelected,
    props.selectedSessionIDs.length,
    selectableSessions.length,
    props.capabilities?.acp_max_sessions ?? 10,
  );
  const content = (
    <CustomizerContent
      sourceOptions={sourceOptions}
      selectedSources={props.selectedSources}
      selectedSessionIDs={props.selectedSessionIDs}
      sessions={props.sessions}
      sessionsLoading={props.sessionsLoading}
      acpSelected={acpSelected}
      selectableSessions={selectableSessions}
      maxSessions={props.capabilities?.acp_max_sessions ?? 10}
      onSourcesChange={props.onSourcesChange}
      onSessionChange={props.onSessionChange}
      onSessionSelectionChange={props.onSessionSelectionChange}
    />
  );
  const footer = (
    <CustomizerFooter
      submitting={props.submitting}
      invalid={validationKey !== null}
      onCancel={() => props.onOpenChange(false)}
      onSubmit={props.onSubmit}
    />
  );
  const validation = validationKey ? (
    <p className="mt-3 text-sm text-destructive" role="alert">
      {validationMessage(validationKey, t)}
    </p>
  ) : null;

  if (isMobile) {
    return (
      <Drawer open={props.open} onOpenChange={props.onOpenChange}>
        <DrawerContent
          data-testid="diagnostic-bundle-drawer"
          className="h-[min(88dvh,680px)] max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] min-w-0 overflow-hidden p-0"
        >
          <DrawerHeader className="shrink-0 border-b border-border px-4 py-3 text-left">
            <DrawerTitle>{t("settings:diagnosticCustomizerTitle")}</DrawerTitle>
            <DrawerDescription>{t("settings:diagnosticCustomizerDescription")}</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4">
            {content}
            {validation}
          </div>
          {footer}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        data-testid="diagnostic-bundle-dialog"
        className="flex max-h-[88dvh] w-[calc(100vw-2rem)] max-w-[calc(100vw-2rem)] min-w-0 flex-col overflow-hidden p-0 sm:max-w-5xl"
      >
        <DialogHeader className="shrink-0 border-b border-border px-4 py-3 pr-12 text-left">
          <DialogTitle>{t("settings:diagnosticCustomizerTitle")}</DialogTitle>
          <DialogDescription>{t("settings:diagnosticCustomizerDescription")}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 py-4">
          {content}
          {validation}
        </div>
        {footer}
      </DialogContent>
    </Dialog>
  );
}

type CustomizerContentProps = {
  sourceOptions: DiagnosticBundleSource[];
  selectedSources: DiagnosticBundleSource[];
  selectedSessionIDs: string[];
  sessions: DiagnosticSession[];
  sessionsLoading: boolean;
  acpSelected: boolean;
  selectableSessions: DiagnosticSession[];
  maxSessions: number;
  onSourcesChange: (sources: DiagnosticBundleSource[]) => void;
  onSessionChange: (sessionID: string, checked: boolean) => void;
  onSessionSelectionChange: (sessionIDs: string[]) => void;
};

function CustomizerContent({
  sourceOptions,
  selectedSources,
  selectedSessionIDs,
  sessions,
  sessionsLoading,
  acpSelected,
  selectableSessions,
  maxSessions,
  onSourcesChange,
  onSessionChange,
  onSessionSelectionChange,
}: CustomizerContentProps) {
  const { t } = useTranslation();

  return (
    <div className="space-y-6">
      {acpSelected && (
        <Alert variant="destructive">
          <AlertTitle>{t("settings:diagnosticSourceACP")}</AlertTitle>
          <AlertDescription>{t("settings:diagnosticACPWarning")}</AlertDescription>
        </Alert>
      )}
      <SourceSection
        sourceOptions={sourceOptions}
        selectedSources={selectedSources}
        onSourcesChange={onSourcesChange}
      />

      {acpSelected && (
        <SessionSection
          sessions={sessions}
          sessionsLoading={sessionsLoading}
          selectableSessions={selectableSessions}
          selectedSessionIDs={selectedSessionIDs}
          maxSessions={maxSessions}
          onSessionChange={onSessionChange}
          onSessionSelectionChange={onSessionSelectionChange}
        />
      )}
    </div>
  );
}

type CustomizerFooterProps = {
  submitting: boolean;
  invalid: boolean;
  onCancel: () => void;
  onSubmit: () => void;
};

function CustomizerFooter({ submitting, invalid, onCancel, onSubmit }: CustomizerFooterProps) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 justify-end gap-2 border-t border-border px-4 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]">
      <Button
        type="button"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onCancel}
        disabled={submitting}
      >
        {t("settings:diagnosticBundleCancel")}
      </Button>
      <Button
        type="button"
        className="min-h-11 cursor-pointer"
        disabled={submitting || invalid}
        onClick={onSubmit}
        data-testid="create-custom-diagnostic-bundle"
      >
        {submitting && <Spinner className="mr-2 size-4" />}
        {t("settings:diagnosticBundleCreate")}
      </Button>
    </div>
  );
}

type SourceSectionProps = {
  sourceOptions: DiagnosticBundleSource[];
  selectedSources: DiagnosticBundleSource[];
  onSourcesChange: (sources: DiagnosticBundleSource[]) => void;
};

function SourceSection({ sourceOptions, selectedSources, onSourcesChange }: SourceSectionProps) {
  const { t } = useTranslation();
  const setSource = (source: DiagnosticBundleSource, checked: boolean) => {
    const next = checked
      ? [...selectedSources, source]
      : selectedSources.filter((selected) => selected !== source);
    onSourcesChange(SOURCE_ORDER.filter((candidate) => next.includes(candidate)));
  };
  return (
    <section aria-labelledby="diagnostic-sources-heading" className="space-y-3">
      <h3 id="diagnostic-sources-heading" className="text-sm font-semibold">
        {t("settings:diagnosticSourcesTitle")}
      </h3>
      <div className="space-y-2">
        {sourceOptions.map((source) => (
          <SourceRow
            key={source}
            source={source}
            checked={selectedSources.includes(source)}
            onCheckedChange={(checked) => setSource(source, checked)}
          />
        ))}
      </div>
    </section>
  );
}

function SourceRow({
  source,
  checked,
  onCheckedChange,
}: {
  source: DiagnosticBundleSource;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const copy = SOURCE_COPY[source];
  return (
    <label className="flex min-h-11 cursor-pointer items-start gap-3 rounded-lg border border-border/70 p-3 transition-colors hover:bg-muted/50">
      <Checkbox
        checked={checked}
        onCheckedChange={(value) => onCheckedChange(value === true)}
        className="mt-0.5"
        aria-label={t(copy.label)}
      />
      <span className="min-w-0">
        <span className="block text-sm font-medium">{t(copy.label)}</span>
        <span className="block text-xs text-muted-foreground">{t(copy.description)}</span>
      </span>
    </label>
  );
}

type SessionSectionProps = {
  sessions: DiagnosticSession[];
  sessionsLoading: boolean;
  selectableSessions: DiagnosticSession[];
  selectedSessionIDs: string[];
  maxSessions: number;
  onSessionChange: (sessionID: string, checked: boolean) => void;
  onSessionSelectionChange: (sessionIDs: string[]) => void;
};

function SessionSection({
  sessions,
  sessionsLoading,
  selectableSessions,
  selectedSessionIDs,
  maxSessions,
  onSessionChange,
  onSessionSelectionChange,
}: SessionSectionProps) {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="diagnostic-sessions-heading" className="space-y-3">
      <div>
        <h3 id="diagnostic-sessions-heading" className="text-sm font-semibold">
          {t("settings:diagnosticSelectSessions")}
        </h3>
        <p className="text-xs text-muted-foreground">
          {t("settings:diagnosticSessionsDescription", { count: maxSessions })}
        </p>
      </div>
      <SessionResults
        sessions={sessions}
        sessionsLoading={sessionsLoading}
        selectableSessions={selectableSessions}
        selectedSessionIDs={selectedSessionIDs}
        maxSessions={maxSessions}
        onSessionChange={onSessionChange}
        onSessionSelectionChange={onSessionSelectionChange}
      />
    </section>
  );
}

function SessionResults({
  sessions,
  sessionsLoading,
  selectableSessions,
  selectedSessionIDs,
  maxSessions,
  onSessionChange,
  onSessionSelectionChange,
}: SessionSectionProps) {
  const { t } = useTranslation();
  if (sessionsLoading) {
    return (
      <div className="flex min-h-11 items-center gap-2 text-sm text-muted-foreground">
        <Spinner className="size-4" />
        {t("settings:diagnosticSessionsLoading")}
      </div>
    );
  }
  if (sessions.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground">
        {t("settings:diagnosticACPNoSessions")}
      </p>
    );
  }
  const selectAll = () => {
    onSessionSelectionChange(
      selectableSessions.slice(0, maxSessions).map((session) => session.session_id),
    );
  };
  return (
    <div className="space-y-2">
      <SessionSelectionActions
        hasSelectableSessions={selectableSessions.length > 0}
        hasSelectedSessions={selectedSessionIDs.length > 0}
        onSelectAll={selectAll}
        onClear={() => onSessionSelectionChange([])}
      />
      {sessions.map((session) => (
        <SessionRow
          key={session.session_id}
          session={session}
          selectable={selectableSessions.includes(session)}
          checked={selectedSessionIDs.includes(session.session_id)}
          maxReached={selectedSessionIDs.length >= maxSessions}
          onSessionChange={onSessionChange}
        />
      ))}
      <p className="text-xs text-muted-foreground">
        {t("settings:diagnosticSelectedSessions", { count: selectedSessionIDs.length })}
      </p>
    </div>
  );
}

function SessionSelectionActions({
  hasSelectableSessions,
  hasSelectedSessions,
  onSelectAll,
  onClear,
}: {
  hasSelectableSessions: boolean;
  hasSelectedSessions: boolean;
  onSelectAll: () => void;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row">
      <Button
        type="button"
        variant="outline"
        className="min-h-11 cursor-pointer sm:w-auto"
        disabled={!hasSelectableSessions}
        onClick={onSelectAll}
        data-testid="select-all-acp-sessions"
      >
        {t("settings:diagnosticSelectAllACPSessions")}
      </Button>
      <Button
        type="button"
        variant="ghost"
        className="min-h-11 cursor-pointer sm:w-auto"
        disabled={!hasSelectedSessions}
        onClick={onClear}
        data-testid="clear-acp-session-selection"
      >
        {t("settings:diagnosticClearACPSelection")}
      </Button>
    </div>
  );
}

function SessionRow({
  session,
  selectable,
  checked,
  maxReached,
  onSessionChange,
}: {
  session: DiagnosticSession;
  selectable: boolean;
  checked: boolean;
  maxReached: boolean;
  onSessionChange: (sessionID: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const title = session.agent || session.provider || session.session_id;
  const taskTitle = session.task_title || session.task_id;
  const details =
    [session.model, session.status, session.executor_type].filter(Boolean).join(" · ") ||
    session.session_id;
  const checkboxID = `diagnostic-acp-session-${session.session_id}`;
  return (
    <div className="flex min-h-11 items-start gap-3 rounded-lg border border-border/70 p-3 transition-colors hover:bg-muted/50">
      <Checkbox
        id={checkboxID}
        checked={checked}
        disabled={!selectable || (!checked && maxReached)}
        onCheckedChange={(value) => onSessionChange(session.session_id, value === true)}
        className="mt-0.5"
        aria-label={session.session_id}
      />
      <div className="min-w-0 flex-1">
        <a
          href={linkToTask(session.task_id)}
          target="_blank"
          rel="noopener noreferrer"
          className="flex w-fit max-w-full items-center gap-1 truncate text-sm font-medium text-foreground underline-offset-4 hover:underline focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={t("settings:diagnosticOpenTask", { title: taskTitle })}
          data-testid={`acp-session-task-link-${session.session_id}`}
        >
          <span className="truncate">{taskTitle}</span>
          <IconExternalLink className="size-3 shrink-0" />
        </a>
        <label htmlFor={checkboxID} className="mt-1 block min-h-11 cursor-pointer">
          <span className="block truncate text-sm font-medium">{title}</span>
          <span className="block truncate text-xs text-muted-foreground">{details}</span>
        </label>
      </div>
      <span className="mt-0.5 shrink-0 text-xs text-muted-foreground">
        {availabilityLabel(session.acp_availability, t)}
      </span>
    </div>
  );
}

function validationMessageKey(
  sources: DiagnosticBundleSource[],
  acpSelected: boolean,
  selectedSessionCount: number,
  selectableSessionCount: number,
  maxSessions: number,
): string | null {
  if (sources.length === 0) return "diagnosticSourceRequired";
  if (acpSelected && selectedSessionCount > maxSessions) return "diagnosticACPSelectionLimit";
  if (acpSelected && (selectableSessionCount === 0 || selectedSessionCount === 0)) {
    return "diagnosticACPSelectionRequired";
  }
  return null;
}

function availabilityLabel(
  availability: DiagnosticSession["acp_availability"],
  t: (key: string) => string,
): string {
  if (availability === "reachable") return t("settings:diagnosticSessionReachable");
  if (availability === "host_retained") return t("settings:diagnosticSessionRetained");
  return t("settings:diagnosticSessionUnavailable");
}

function validationMessage(key: string, t: (key: string) => string): string {
  if (key === "diagnosticSourceRequired") return t("settings:diagnosticSourceRequired");
  if (key === "diagnosticACPSelectionRequired") {
    return t("settings:diagnosticACPSelectionRequired");
  }
  return t("settings:diagnosticACPSelectionLimit");
}
