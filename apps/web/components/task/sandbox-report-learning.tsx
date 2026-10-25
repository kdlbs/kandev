"use client";

import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Dialog, DialogTrigger } from "@kandev/ui/dialog";
import { useSandboxLearning } from "@/hooks/domains/session/use-sandbox-learning";
import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import { SandboxLearningReview } from "./sandbox-learning-review";

export function SandboxReportLearning({
  sessionId,
  report,
}: {
  sessionId: string;
  report: SandboxReport;
}) {
  const { t } = useTranslation();
  const prefix = useId();
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const model = useSandboxLearning(sessionId, report);
  const records = report.data.records ?? [];
  if (records.length === 0) return <p>{t("agents:reportEmpty")}</p>;
  return (
    <div className="min-w-0 space-y-3">
      <ul className="min-w-0 divide-y">
        {records.map((record) => (
          <li key={record.id} className="flex items-start gap-3 py-3">
            {model.canPropose && (
              <Checkbox
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
            )}
            <label
              htmlFor={model.canPropose ? `${prefix}-${record.id}` : undefined}
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
            disabled={selectedIds.length === 0}
          >
            {t(model.revision ? "agents:learningViewRevision" : "agents:learningReviewSelected")}
          </Button>
        </DialogTrigger>
        <SandboxLearningReview model={model} selectedIds={selectedIds} />
      </Dialog>
    </div>
  );
}
