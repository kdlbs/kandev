"use client";

import { useEffect, useState } from "react";
import { IconExternalLink, IconLoader2, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
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
import { useTranslation } from "react-i18next";
import type { CursorCloudSubmissionResolution } from "@/lib/api/domains/cursor-cloud-api";

export type CursorCloudSubmissionRecoveryState =
  | "loading"
  | "ready"
  | "unknown"
  | "pending"
  | "error";

type CursorCloudSubmissionRecoveryProps = {
  state: CursorCloudSubmissionRecoveryState;
  resolution: CursorCloudSubmissionResolution | null;
  isMobile: boolean;
  agentUrl: string | null;
  busy: boolean;
  error?: string | null;
  onBind: (runId: string) => void;
  onRetry: (operationId: string) => void;
  onRefresh: () => void;
};

function SubmissionRecoveryStatus({
  state,
  error,
  onRefresh,
}: Pick<CursorCloudSubmissionRecoveryProps, "state" | "error" | "onRefresh">) {
  const { t } = useTranslation();
  if (state === "ready") return null;

  if (state === "loading" || state === "pending") {
    return (
      <div
        role="status"
        className="flex items-center gap-2 px-4 py-2 text-sm text-muted-foreground"
      >
        <IconLoader2 className="size-4 animate-spin" />
        {t("executors:cursorCloudCheckingSubmission")}
      </div>
    );
  }

  if (state === "error") {
    return (
      <div
        role="alert"
        className="flex flex-wrap items-center gap-2 px-4 py-2 text-sm text-destructive"
      >
        <span>{error || t("executors:cursorCloudSubmissionCheckFailed")}</span>
        <Button type="button" variant="outline" className="min-h-11" onClick={onRefresh}>
          {t("executors:cursorCloudCheckAgain")}
        </Button>
      </div>
    );
  }

  return null;
}

function ResolutionContents({
  resolution,
  busy,
  onClose,
  onBind,
  onRetry,
}: Pick<CursorCloudSubmissionRecoveryProps, "resolution" | "busy" | "onBind" | "onRetry"> & {
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const candidates = resolution?.candidates ?? [];
  const [selectedRunId, setSelectedRunId] = useState(candidates[0]?.runId ?? "");
  const [acknowledgeDuplicateWork, setAcknowledgeDuplicateWork] = useState(false);

  useEffect(() => {
    setSelectedRunId(candidates[0]?.runId ?? "");
    setAcknowledgeDuplicateWork(false);
  }, [resolution?.operationId]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 pb-4">
        {candidates.length > 0 ? (
          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">{t("executors:cursorCloudRemoteRuns")}</legend>
            {candidates.map((candidate) => (
              <label
                key={candidate.runId}
                className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md border px-3 py-2 text-sm"
              >
                <input
                  type="radio"
                  name="cursor-cloud-run-candidate"
                  value={candidate.runId}
                  checked={selectedRunId === candidate.runId}
                  onChange={() => setSelectedRunId(candidate.runId)}
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{candidate.runId}</span>
                  <span className="block text-muted-foreground">
                    {t("executors:cursorCloudRunDetails", {
                      status: candidate.status,
                      createdAt: new Date(candidate.createdAt).toLocaleString(),
                    })}
                  </span>
                </span>
              </label>
            ))}
          </fieldset>
        ) : (
          <p className="text-sm text-muted-foreground">{t("executors:cursorCloudNoRemoteRuns")}</p>
        )}
        <label className="flex min-h-11 cursor-pointer items-start gap-3 rounded-md border px-3 py-2 text-sm">
          <input
            className="mt-1"
            type="checkbox"
            checked={acknowledgeDuplicateWork}
            onChange={(event) => setAcknowledgeDuplicateWork(event.currentTarget.checked)}
          />
          <span>{t("executors:cursorCloudRetryAcknowledgment")}</span>
        </label>
      </div>
      <div className="flex shrink-0 flex-col gap-2 border-t p-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))] sm:flex-row sm:justify-end">
        {candidates.length > 0 && (
          <Button
            type="button"
            variant="outline"
            className="min-h-11"
            disabled={busy || !selectedRunId}
            onClick={() => onBind(selectedRunId)}
          >
            {t("executors:cursorCloudBindRun")}
          </Button>
        )}
        <Button
          type="button"
          className="min-h-11"
          disabled={busy || !acknowledgeDuplicateWork || !resolution?.operationId}
          onClick={() => resolution?.operationId && onRetry(resolution.operationId)}
        >
          {t("executors:cursorCloudRetrySubmission")}
        </Button>
        <Button type="button" variant="ghost" className="min-h-11" onClick={onClose}>
          {t("common:close")}
        </Button>
      </div>
    </div>
  );
}

export function CursorCloudSubmissionRecovery({
  state,
  resolution,
  isMobile,
  agentUrl,
  busy,
  error,
  onBind,
  onRetry,
  onRefresh,
}: CursorCloudSubmissionRecoveryProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  if (state !== "unknown")
    return <SubmissionRecoveryStatus state={state} error={error} onRefresh={onRefresh} />;

  const content = (
    <>
      {isMobile ? (
        <DrawerHeader className="flex shrink-0 flex-row items-start justify-between border-b px-4 py-3 text-left">
          <div className="min-w-0">
            <DrawerTitle>{t("executors:cursorCloudResolveSubmission")}</DrawerTitle>
            <DrawerDescription>
              {t("executors:cursorCloudResolveSubmissionDescription")}
            </DrawerDescription>
          </div>
          <Button
            type="button"
            variant="ghost"
            className="size-11 shrink-0 p-0"
            aria-label={t("common:close")}
            onClick={() => setOpen(false)}
          >
            <IconX className="size-4" />
          </Button>
        </DrawerHeader>
      ) : (
        <DialogHeader>
          <DialogTitle>{t("executors:cursorCloudResolveSubmission")}</DialogTitle>
          <DialogDescription>
            {t("executors:cursorCloudResolveSubmissionDescription")}
          </DialogDescription>
        </DialogHeader>
      )}
      <ResolutionContents
        resolution={resolution}
        busy={busy}
        onBind={onBind}
        onRetry={onRetry}
        onClose={() => setOpen(false)}
      />
    </>
  );

  return (
    <>
      <div
        role="alert"
        data-testid="cursor-cloud-submission-unknown"
        className="flex flex-wrap items-center gap-2 border-b border-amber-500/40 bg-amber-500/10 px-4 py-2 text-sm"
      >
        <span className="min-w-0 flex-1">{t("executors:cursorCloudSubmissionUnknown")}</span>
        {agentUrl && (
          <a
            className="inline-flex min-h-11 items-center gap-1 px-2 font-medium underline"
            href={agentUrl}
            target="_blank"
            rel="noreferrer"
          >
            {t("executors:cursorCloudOpenInCursor")}
            <IconExternalLink className="size-4" />
          </a>
        )}
        <Button
          type="button"
          variant="outline"
          className="min-h-11"
          disabled={busy}
          onClick={() => setOpen(true)}
        >
          {t("executors:cursorCloudResolveSubmission")}
        </Button>
      </div>
      {isMobile ? (
        <Drawer open={open} onOpenChange={setOpen}>
          <DrawerContent className="flex h-[100dvh] max-h-[100dvh] flex-col overflow-hidden rounded-none pb-0">
            {content}
          </DrawerContent>
        </Drawer>
      ) : (
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogContent className="flex max-h-[85dvh] flex-col overflow-hidden p-0 sm:max-w-xl">
            <div className="px-6 pt-6">{content}</div>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}
