"use client";

import { useId, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Dialog, DialogTrigger } from "@kandev/ui/dialog";
import { useSandboxLearning } from "@/hooks/domains/session/use-sandbox-learning";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import type { SandboxDenialVisibility } from "@/hooks/domains/session/use-sandbox-denial-visibility";
import { SandboxLearningReview } from "./sandbox-learning-review";
import { SandboxDenialVisibilityControls } from "./sandbox-denial-visibility-controls";

export function SandboxReportLearning({
  sessionId,
  report,
  visibility,
}: {
  sessionId: string;
  report: SandboxReport;
  visibility: SandboxDenialVisibility;
}) {
  const { t } = useTranslation();
  const prefix = useId();
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const model = useSandboxLearning(sessionId, report);
  const records = visibility.records;
  const firstRecord = useRef<HTMLButtonElement>(null);
  const focusAfterShow = useRef(false);
  useLayoutEffect(() => {
    if (focusAfterShow.current && records.length > 0) {
      firstRecord.current?.focus();
      focusAfterShow.current = false;
    }
  }, [records]);
  if (!report.data.records?.length) return <p>{t("agents:reportEmpty")}</p>;
  return (
    <div className="min-w-0 space-y-3">
      <SandboxDenialVisibilityControls
        visibility={visibility}
        selectedIds={selectedIds}
        onHide={() => setSelectedIds([])}
        onShow={() => {
          focusAfterShow.current = true;
        }}
      />
      <ul className="min-w-0 divide-y">
        {records.map((record, index) => (
          <li key={record.id} className="flex items-start gap-3 py-3">
            <Checkbox
              ref={index === 0 ? firstRecord : undefined}
              id={`${prefix}-${record.id}`}
              className="mt-1"
              checked={selectedIds.includes(record.id)}
              disabled={!selectedIds.includes(record.id) && selectedIds.length >= 256}
              onCheckedChange={(checked) =>
                setSelectedIds((ids) =>
                  checked ? [...ids, record.id] : ids.filter((id) => id !== record.id),
                )
              }
            />
            <label
              htmlFor={`${prefix}-${record.id}`}
              className="min-w-0 flex-1 space-y-1 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer"
            >
              <span className="block font-medium">
                {t(`agents:reportOperation_${record.operation}`)}
              </span>
              <span className="block break-all font-mono">{record.target}</span>
              <span className="block text-muted-foreground break-words">{record.occurred_at}</span>
            </label>
          </li>
        ))}
      </ul>
      {!model.canPropose && selectedIds.length === 0 && (
        <p className="text-muted-foreground">{t("agents:learningUnavailable")}</p>
      )}
      <Dialog>
        <DialogTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 min-h-7 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer"
            disabled={selectedIds.length === 0 || (!model.canPropose && !model.revision)}
          >
            {t(model.revision ? "agents:learningViewRevision" : "agents:learningReviewSelected")}
          </Button>
        </DialogTrigger>
        <SandboxLearningReview model={model} selectedIds={selectedIds} />
      </Dialog>
    </div>
  );
}
