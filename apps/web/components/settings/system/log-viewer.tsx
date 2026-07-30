"use client";

import { useEffect, useRef, useState } from "react";
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
        throw new Error(job.warnings?.[0] ?? "The diagnostic bundle could not be prepared.");
      }
      setState(job.status === "partial" ? "partial" : "idle");
      setMessage(bundleMessage(job));
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
      setMessage(error instanceof Error ? error.message : "The diagnostic bundle failed.");
    }
  };

  const pending = state === "collecting" || state === "preparing";
  return (
    <div className="min-w-0 space-y-4">
      <Alert>
        <IconAlertTriangle className="size-4" />
        <AlertTitle>Review before sharing</AlertTitle>
        <AlertDescription>
          This ZIP combines up to three days of backend logs with locally retained frontend console
          events from connected browsers. It may contain URLs, console arguments, stacks, paths,
          runtime metadata, and user-visible errors.
        </AlertDescription>
      </Alert>

      <Card data-testid="system-diagnostic-bundle-card">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <IconFileZip className="size-4" />
            Frontend + backend diagnostic logs
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Kandev asks your connected browser tabs for their bounded three-day console history,
            combines it with retained backend log files, and downloads one ZIP.
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
            {buttonLabel(state)}
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

function buttonLabel(state: ViewState): string {
  if (state === "collecting") return "Collecting frontend logs…";
  if (state === "preparing") return "Preparing ZIP…";
  return "Download diagnostic bundle";
}

function bundleMessage(job: DiagnosticBundleJob): string {
  if (job.status !== "partial") return "Your diagnostic ZIP is downloading.";
  return job.warnings?.length
    ? `A partial ZIP is downloading: ${job.warnings.join(" ")}`
    : "A partial diagnostic ZIP is downloading; some frontend logs were unavailable.";
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
