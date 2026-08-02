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
  fetchDiagnosticBundle,
} from "@/lib/api/domains/system-api";
import type { DiagnosticBundleJob } from "@/lib/types/system";

type ViewState = "idle" | "collecting" | "preparing" | "partial" | "busy" | "error";

export function LogViewer() {
  const { t } = useTranslation();
  const [state, setState] = useState<ViewState>("idle");
  const [message, setMessage] = useState("");
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const download = async () => {
    setMessage("");
    setState("collecting");
    try {
      const job = await prepareDiagnosticBundle((next) => mounted.current && setState(next));
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
        setMessage(`Diagnostics are busy. Try again in ${retry} seconds.`);
        window.setTimeout(() => mounted.current && setState("idle"), retry * 1_000);
        return;
      }
      setState("error");
      setMessage(error instanceof Error ? error.message : t("settings:diagnosticBundleFailed"));
    }
  };

  const pending = state === "collecting" || state === "preparing";
  return (
    <div className="min-w-0 space-y-4">
      <Alert>
        <IconAlertTriangle className="size-4" />
        <AlertTitle>{t("settings:diagnosticReviewBeforeSharing")}</AlertTitle>
        <AlertDescription>{t("settings:diagnosticReviewDescription")}</AlertDescription>
      </Alert>

      <Card data-testid="system-diagnostic-bundle-card">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconFileZip className="size-4" />
            {t("settings:diagnosticBundleTitle")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {t("settings:diagnosticBundleDescription")}
          </p>
          <Button
            className="min-h-11 w-full cursor-pointer sm:w-auto"
            disabled={pending}
            onClick={() => void download()}
            data-testid="download-diagnostic-bundle"
          >
            {pending ? (
              <Spinner className="mr-2 size-4" />
            ) : (
              <IconDownload className="mr-2 size-4" />
            )}
            {buttonLabel(state, t)}
          </Button>
          {message && (
            <p
              className={
                state === "error" ? "text-sm text-destructive" : "text-sm text-muted-foreground"
              }
              role={state === "error" ? "alert" : "status"}
              data-testid="diagnostic-bundle-status"
            >
              {message}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

async function prepareDiagnosticBundle(onStatus: (state: ViewState) => void) {
  let job = await createDiagnosticBundle(["backend", "frontend"]);
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
  return t("settings:downloadDiagnosticBundle");
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
