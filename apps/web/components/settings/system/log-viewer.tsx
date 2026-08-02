"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconAlertTriangle, IconDownload, IconFileZip } from "@tabler/icons-react";
import { ApiError } from "@/lib/api/client";
import {
  buildDiagnosticBundleDownloadUrl,
  createDiagnosticBundle,
  fetchDiagnosticACPSessions,
  fetchDiagnosticBundle,
  fetchDiagnosticBundleCapabilities,
} from "@/lib/api/domains/system-api";
import type {
  DiagnosticBundleCapabilities,
  DiagnosticBundleJob,
  DiagnosticBundleSource,
  DiagnosticSession,
} from "@/lib/types/system";
import { BundleCustomizer } from "./bundle-customizer";

type ViewState = "idle" | "collecting" | "preparing" | "partial" | "busy" | "error";
type CustomizerMode = "standard" | "acp";

const STANDARD_SOURCES: DiagnosticBundleSource[] = ["backend", "frontend"];
const CUSTOM_SOURCES: DiagnosticBundleSource[] = ["backend", "frontend", "runtime"];

// eslint-disable-next-line max-lines-per-function -- coordinates bundle polling and shared customizer state.
export function LogViewer() {
  const { t } = useTranslation();
  const [state, setState] = useState<ViewState>("idle");
  const [message, setMessage] = useState("");
  const [capabilities, setCapabilities] = useState<DiagnosticBundleCapabilities | null>(null);
  const [sessions, setSessions] = useState<DiagnosticSession[]>([]);
  const [sessionsLoading, setSessionsLoading] = useState(false);
  const [sessionsLoaded, setSessionsLoaded] = useState(false);
  const [customizerOpen, setCustomizerOpen] = useState(false);
  const [customizerMode, setCustomizerMode] = useState<CustomizerMode>("standard");
  const [selectedSources, setSelectedSources] = useState<DiagnosticBundleSource[]>(CUSTOM_SOURCES);
  const [selectedSessionIDs, setSelectedSessionIDs] = useState<string[]>([]);
  const [customizerSubmitting, setCustomizerSubmitting] = useState(false);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    void fetchDiagnosticBundleCapabilities()
      .then((next) => mounted.current && setCapabilities(next))
      .catch(() => undefined);
    return () => {
      mounted.current = false;
    };
  }, []);

  useEffect(() => {
    if (!customizerOpen || customizerMode !== "acp" || sessionsLoaded) return;
    setSessionsLoading(true);
    void fetchDiagnosticACPSessions()
      .then((next) => {
        if (!mounted.current) return;
        setSessions(next);
        setSessionsLoaded(true);
      })
      .catch(() => {
        if (!mounted.current) return;
        setSessions([]);
        setSessionsLoaded(true);
      })
      .finally(() => mounted.current && setSessionsLoading(false));
  }, [customizerMode, customizerOpen, sessionsLoaded]);

  const download = async (sources: DiagnosticBundleSource[], sessionIDs: string[] = []) => {
    setMessage("");
    setState("collecting");
    try {
      const job = await prepareDiagnosticBundle(sources, sessionIDs, (next) => {
        if (mounted.current) setState(next);
      });
      if (!mounted.current) return;
      if (job.status !== "ready" && job.status !== "partial") {
        throw new Error(job.warnings?.[0] ?? t("settings:diagnosticBundlePrepareError"));
      }
      setState(job.status === "partial" ? "partial" : "idle");
      setMessage(bundleMessage(job, t));
      triggerDownload(buildDiagnosticBundleDownloadUrl(job.id));
    } catch (error) {
      if (!mounted.current) return;
      if (error instanceof ApiError && (error.status === 429 || error.status === 503)) {
        const retry = error.retryAfterSeconds ?? 5;
        setState("busy");
        setMessage(t("settings:diagnosticBusy", { seconds: retry }));
        window.setTimeout(() => mounted.current && setState("idle"), retry * 1_000);
        return;
      }
      setState("error");
      setMessage(error instanceof Error ? error.message : t("settings:diagnosticBundleFailed"));
    }
  };

  const openCustomizer = (mode: CustomizerMode) => {
    const available = new Set(capabilities?.sources ?? CUSTOM_SOURCES);
    const defaults = mode === "acp" ? [...CUSTOM_SOURCES, "acp" as const] : CUSTOM_SOURCES;
    setCustomizerMode(mode);
    setSelectedSources(defaults.filter((source) => available.has(source)));
    setSelectedSessionIDs([]);
    setCustomizerSubmitting(false);
    setCustomizerOpen(true);
  };

  const submitCustomizer = () => {
    setCustomizerSubmitting(true);
    setCustomizerOpen(false);
    void download(selectedSources, selectedSessionIDs).finally(() => {
      if (mounted.current) setCustomizerSubmitting(false);
    });
  };

  const pending = state === "collecting" || state === "preparing";
  const acpAvailable =
    capabilities?.acp_debug_enabled === true && capabilities.sources.includes("acp");

  return (
    <div className="min-w-0 space-y-4">
      <DiagnosticDisclosure />
      <DiagnosticBundleCard
        pending={pending}
        state={state}
        message={message}
        acpAvailable={acpAvailable}
        onDownloadStandard={() => void download(STANDARD_SOURCES)}
        onCustomize={() => openCustomizer("standard")}
        onDownloadWithACP={() => openCustomizer("acp")}
      />

      <BundleCustomizer
        open={customizerOpen}
        onOpenChange={setCustomizerOpen}
        mode={customizerMode}
        capabilities={capabilities}
        sessions={sessions}
        sessionsLoading={sessionsLoading}
        selectedSources={selectedSources}
        selectedSessionIDs={selectedSessionIDs}
        submitting={customizerSubmitting}
        onSourcesChange={(sources) => {
          setSelectedSources(sources);
          if (!sources.includes("acp")) setSelectedSessionIDs([]);
        }}
        onSessionChange={(sessionID, checked) =>
          setSelectedSessionIDs((current) => updateSessionSelection(current, sessionID, checked))
        }
        onSubmit={submitCustomizer}
      />
    </div>
  );
}

function DiagnosticDisclosure() {
  const { t } = useTranslation();
  return (
    <Alert>
      <IconAlertTriangle className="size-4" />
      <AlertTitle>{t("settings:diagnosticReviewBeforeSharing")}</AlertTitle>
      <AlertDescription className="space-y-2">
        <p>{t("settings:diagnosticReviewDescription")}</p>
        <p>{t("settings:diagnosticNoMessages")}</p>
        <p>{t("settings:diagnosticIncidentalText")}</p>
      </AlertDescription>
    </Alert>
  );
}

type DiagnosticBundleCardProps = {
  pending: boolean;
  state: ViewState;
  message: string;
  acpAvailable: boolean;
  onDownloadStandard: () => void;
  onCustomize: () => void;
  onDownloadWithACP: () => void;
};

function DiagnosticBundleCard({
  pending,
  state,
  message,
  acpAvailable,
  onDownloadStandard,
  onCustomize,
  onDownloadWithACP,
}: DiagnosticBundleCardProps) {
  const { t } = useTranslation();
  return (
    <Card data-testid="system-diagnostic-bundle-card">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <IconFileZip className="size-4" />
          {t("settings:diagnosticBundleTitle")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">{t("settings:diagnosticBundleDescription")}</p>
        <div className="grid gap-2 text-xs text-muted-foreground sm:grid-cols-2">
          <p>{t("settings:diagnosticBackendEvents")}</p>
          <p>{t("settings:diagnosticFrontendEvents")}</p>
        </div>
        <DiagnosticBundleActions
          pending={pending}
          state={state}
          acpAvailable={acpAvailable}
          onDownloadStandard={onDownloadStandard}
          onCustomize={onCustomize}
          onDownloadWithACP={onDownloadWithACP}
        />
        {message && <DiagnosticBundleMessage state={state} message={message} />}
      </CardContent>
    </Card>
  );
}

function DiagnosticBundleActions({
  pending,
  state,
  acpAvailable,
  onDownloadStandard,
  onCustomize,
  onDownloadWithACP,
}: Omit<DiagnosticBundleCardProps, "message">) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
      <Button
        className="min-h-11 w-full cursor-pointer sm:w-auto"
        disabled={pending}
        onClick={onDownloadStandard}
        data-testid="download-diagnostic-bundle"
      >
        {pending ? <Spinner className="mr-2 size-4" /> : <IconDownload className="mr-2 size-4" />}
        {pending ? buttonLabel(state, t) : t("settings:diagnosticDownloadStandard")}
      </Button>
      <Button
        variant="outline"
        className="min-h-11 w-full cursor-pointer sm:w-auto"
        disabled={pending}
        onClick={onCustomize}
        data-testid="customize-diagnostic-bundle"
      >
        {t("settings:diagnosticCustomize")}
      </Button>
      {acpAvailable && (
        <Button
          variant="outline"
          className="min-h-11 w-full cursor-pointer sm:w-auto"
          disabled={pending}
          onClick={onDownloadWithACP}
          data-testid="download-diagnostic-bundle-with-acp"
        >
          {t("settings:diagnosticDownloadWithACP")}
        </Button>
      )}
    </div>
  );
}

function DiagnosticBundleMessage({ state, message }: { state: ViewState; message: string }) {
  return (
    <p
      className={state === "error" ? "text-sm text-destructive" : "text-sm text-muted-foreground"}
      role={state === "error" ? "alert" : "status"}
      data-testid="diagnostic-bundle-status"
    >
      {message}
    </p>
  );
}

function updateSessionSelection(current: string[], sessionID: string, checked: boolean): string[] {
  if (!checked) return current.filter((id) => id !== sessionID);
  if (current.includes(sessionID)) return current;
  return [...current, sessionID];
}

async function prepareDiagnosticBundle(
  sources: DiagnosticBundleSource[],
  sessionIDs: string[],
  onStatus: (state: ViewState) => void,
) {
  let job = await createDiagnosticBundle(sources, sessionIDs);
  while (job.status === "collecting" || job.status === "building") {
    onStatus(job.status === "collecting" ? "collecting" : "preparing");
    await delay(500);
    job = await fetchDiagnosticBundle(job.id);
  }
  return job;
}

function buttonLabel(state: ViewState, t: (key: string) => string): string {
  if (state === "collecting") return t("settings:diagnosticCollectingFrontendLogs");
  if (state === "preparing") return t("settings:diagnosticPreparingZip");
  return t("settings:diagnosticDownloadStandard");
}

function bundleMessage(
  job: DiagnosticBundleJob,
  t: (key: string, options?: Record<string, unknown>) => string,
): string {
  if (job.status !== "partial") return t("settings:diagnosticZipDownloading");
  return job.warnings?.length
    ? t("settings:diagnosticPartialZipDownloading", { warnings: job.warnings.join(" ") })
    : t("settings:diagnosticPartialZipUnavailable");
}

function triggerDownload(url: string): void {
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = "kandev-diagnostic-logs.zip";
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, milliseconds));
}
