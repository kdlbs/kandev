import { IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { formatGigabytes } from "./storage-units";

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function temporaryArtifactResult(result: Record<string, unknown>): {
  quarantinedBytes: number;
  failed: number;
} | null {
  const outer = recordValue(result.result) ?? result;
  const provider = recordValue(outer.temporary_artifacts);
  const cleanup = recordValue(provider?.result) ?? provider;
  if (!cleanup) return null;
  const rawBytes = cleanup.quarantined_bytes ?? cleanup.reclaimed_bytes;
  if (typeof rawBytes !== "number" || !Number.isFinite(rawBytes) || rawBytes < 0) return null;
  const rawFailed = cleanup.failed;
  return {
    quarantinedBytes: rawBytes,
    failed: typeof rawFailed === "number" && Number.isFinite(rawFailed) ? rawFailed : 0,
  };
}

export function TemporaryArtifactRunResult({ result }: { result: Record<string, unknown> }) {
  const { t } = useTranslation();
  const cleanup = temporaryArtifactResult(result);
  if (!cleanup) return null;
  return (
    <div className="space-y-1 text-sm" data-testid="storage-temporary-artifacts-result">
      <div>
        {t("system:storageTemporaryArtifactsQuarantinedResult", {
          size: formatGigabytes(cleanup.quarantinedBytes),
        })}
      </div>
      <div className="text-muted-foreground">
        {t("system:storageTemporaryArtifactsFreedAfterDelete")}
      </div>
      {cleanup.failed > 0 && (
        <div className="text-amber-600">{t("system:storageTemporaryArtifactsMoveFailed")}</div>
      )}
    </div>
  );
}

export function StorageRunTechnicalDetails({
  message,
  result,
}: {
  message: string;
  result: Record<string, unknown>;
}) {
  const { t } = useTranslation();
  return (
    <details className="group" data-testid="storage-run-technical-details">
      <summary
        className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 text-sm text-muted-foreground md:min-h-7 [&::-webkit-details-marker]:hidden"
        data-testid="storage-run-technical-trigger"
      >
        <IconChevronRight
          className="size-4 transition-transform group-open:rotate-90"
          aria-hidden="true"
        />
        {t("system:storageHistoryTechnicalDetails")}
      </summary>
      <div
        className="mt-2 max-h-64 space-y-2 overflow-auto rounded-md border bg-muted/40 p-3"
        data-testid="storage-run-technical-content"
      >
        {message && (
          <div
            className="break-words whitespace-pre-wrap text-muted-foreground"
            data-testid="storage-run-technical-message"
          >
            {message}
          </div>
        )}
        <pre
          className="max-w-full whitespace-pre-wrap break-words text-xs"
          data-testid="storage-run-technical-json"
        >
          {JSON.stringify(result, null, 2)}
        </pre>
      </div>
    </details>
  );
}
