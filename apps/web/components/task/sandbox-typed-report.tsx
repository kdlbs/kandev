"use client";

import { type RefObject, useId, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { SandboxDenialVisibilityControls } from "./sandbox-denial-visibility-controls";
import type { SandboxDenialVisibility } from "@/hooks/domains/session/use-sandbox-denial-visibility";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import {
  filterReportEvents,
  reportEventDetails,
  reportLearningIds,
} from "@/lib/sandbox-report-events";
import type {
  SandboxReportDecision,
  SandboxReportEvent,
  SandboxReportKind,
  SandboxTypedReport,
} from "@/lib/api/domains/sandbox-report-types";

const kinds: SandboxReportKind[] = ["filesystem", "network", "command", "http_endpoint"];
const decisions: SandboxReportDecision[] = [
  "allowed",
  "denied",
  "approval_requested",
  "approval_granted",
  "approval_denied",
  "error",
];
const controlClass = "h-7 min-h-7 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer";

export function SandboxTypedReportContent({
  report,
  selectedIds,
  onSelect,
  onClearSelection,
  visibility,
}: {
  onClearSelection: () => void;
  visibility: SandboxDenialVisibility;
  report: SandboxTypedReport;
  selectedIds: string[];
  onSelect: (id: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const [kind, setKind] = useState<SandboxReportKind | "all">("all");
  const [decision, setDecision] = useState<SandboxReportDecision | "all">("all");
  const prefix = useId();
  const events = filterReportEvents(report.records ?? [], kind, decision);
  return (
    <div className="min-w-0 space-y-4" data-testid="sandbox-typed-report">
      <dl className="grid gap-3 md:grid-cols-2">
        {report.coverage.map((value) => (
          <div key={value.kind} className="rounded-md border p-3">
            <dt className="font-medium">{t(`agents:reportClass_${value.kind}`)}</dt>
            <dd className="mt-1 text-muted-foreground">
              {t(`agents:reportCoverage_${value.coverage}`)}
            </dd>
          </div>
        ))}
        <div>
          <dt className="font-medium">{t("agents:executionAuditIntegrity")}</dt>
          <dd>{t(`agents:reportChain_${report.integrity}`)}</dd>
        </div>
        <div>
          <dt className="font-medium">{t("agents:executionAuditFinalization")}</dt>
          <dd>{t(`agents:reportFinalization_${report.finalization}`)}</dd>
        </div>
      </dl>
      <div className="grid gap-3 md:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor={`${prefix}-kind`}>{t("agents:reportKindFilter")}</Label>
          <Select value={kind} onValueChange={(value) => setKind(value as typeof kind)}>
            <SelectTrigger id={`${prefix}-kind`} className={controlClass}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("agents:reportFilterAll")}</SelectItem>
              {kinds.map((value) => (
                <SelectItem key={value} value={value}>
                  {t(`agents:reportClass_${value}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${prefix}-decision`}>{t("agents:reportDecisionFilter")}</Label>
          <Select value={decision} onValueChange={(value) => setDecision(value as typeof decision)}>
            <SelectTrigger id={`${prefix}-decision`} className={controlClass}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t("agents:reportFilterAll")}</SelectItem>
              {decisions.map((value) => (
                <SelectItem key={value} value={value}>
                  {t(`agents:reportDecision_${value}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <TypedReportRows
        events={events}
        selectedIds={selectedIds}
        onSelect={onSelect}
        visibility={visibility}
        onClearSelection={onClearSelection}
      />
    </div>
  );
}

function TypedReportRows({
  events,
  selectedIds,
  onSelect,
  visibility,
  onClearSelection,
}: {
  events: SandboxReportEvent[];
  selectedIds: string[];
  onSelect: (id: string, checked: boolean) => void;
  visibility: SandboxDenialVisibility;
  onClearSelection: () => void;
}) {
  const { t } = useTranslation();
  const first = useRef<HTMLButtonElement>(null);
  const focusAfterShow = useRef(false);
  const visibleIDs = new Set(visibility.records.map((record) => record.id));
  const visible = events.filter(
    (event) => !event.legacy_record_id || visibleIDs.has(event.legacy_record_id),
  );
  useLayoutEffect(() => {
    if (focusAfterShow.current && first.current) {
      first.current.focus();
      focusAfterShow.current = false;
    }
  }, [visible]);
  return (
    <>
      <SandboxDenialVisibilityControls
        visibility={visibility}
        selectedIds={selectedIds}
        onHide={onClearSelection}
        onShow={() => {
          focusAfterShow.current = true;
        }}
      />
      <p role="status">{t("agents:reportEventCount", { count: visible.length })}</p>
      <ul className="min-w-0 divide-y">
        {visible.map((event) => (
          <ReportEventRow
            key={event.id}
            event={event}
            selectedIds={selectedIds}
            onSelect={onSelect}
            first={first}
            isFirst={
              visible.find((value) => reportLearningIds([value]).length > 0)?.id === event.id
            }
          />
        ))}
      </ul>
      {visible.length === 0 && <p>{t("agents:reportNoMatches")}</p>}
    </>
  );
}

function ReportEventRow({
  event,
  selectedIds,
  onSelect,
  first,
  isFirst,
}: {
  first: RefObject<HTMLButtonElement | null>;
  isFirst: boolean;
  event: SandboxReportEvent;
  selectedIds: string[];
  onSelect: (id: string, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const legacyId = reportLearningIds([event])[0];
  const selected = Boolean(legacyId && selectedIds.includes(legacyId));
  return (
    <li className="min-w-0 space-y-2 py-4" data-testid="sandbox-report-event">
      <div className="flex items-start gap-3">
        {legacyId && (
          <Checkbox
            ref={isFirst ? first : undefined}
            id={id}
            className="mt-1"
            checked={selected}
            disabled={!selected && selectedIds.length >= 256}
            onCheckedChange={(checked) => onSelect(legacyId, checked === true)}
          />
        )}
        <div className="min-w-0 flex-1 space-y-2">
          <p className="flex flex-wrap items-center gap-x-2 gap-y-1 font-medium">
            <span>{t(`agents:reportClass_${event.kind}`)}</span>
            <span className="rounded border px-1.5 py-0.5">
              {t(`agents:reportDecision_${event.decision}`)}
            </span>
          </p>
          {legacyId ? (
            <label
              htmlFor={id}
              className="block min-w-0 max-md:min-h-11 pointer-coarse:min-h-11 cursor-pointer"
            >
              {reportEventDetails(event).map((value, index) => (
                <code key={index} className="block break-all">
                  {value}
                </code>
              ))}
            </label>
          ) : (
            <div>
              {reportEventDetails(event).map((value, index) => (
                <code key={index} className="block break-all">
                  {value}
                </code>
              ))}
            </div>
          )}
          {event.learning_blocker && (
            <p className="text-muted-foreground">
              {t(`agents:reportLearningBlocker_${event.learning_blocker}`)}
            </p>
          )}
          <details>
            <summary className={`${controlClass} flex items-center w-fit`}>
              {t("agents:reportEventDetails")}
            </summary>
            <dl className="space-y-2 break-all pt-2 text-muted-foreground">
              <div>
                <dt>{t("agents:reportEventTime")}</dt>
                <dd>{event.occurred_at}</dd>
              </div>
              <div>
                <dt>{t("agents:reportEventSource")}</dt>
                <dd className="font-mono">{event.source_id}</dd>
              </div>
            </dl>
          </details>
        </div>
      </div>
    </li>
  );
}
