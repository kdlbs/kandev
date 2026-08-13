"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { patchTaskPRDisposition } from "@/lib/api/domains/github-api";
import type { TaskPR, TaskPRDisposition } from "@/lib/types/github";

// Sentinel value for the "no disposition recorded" / clear option. Radix
// Select reserves the empty string for its own internal "no selection"
// state, so a real, non-empty sentinel is required here.
const NONE_VALUE = "__none__";

const DISPOSITION_VALUES: TaskPRDisposition[] = [
  "unknown",
  "superseded",
  "duplicate",
  "exploratory",
  "withdrawn",
];

function dispositionLabel(t: (key: string) => string, value: TaskPRDisposition): string {
  switch (value) {
    case "unknown":
      return t("github:dispositionUnknown");
    case "superseded":
      return t("github:dispositionSuperseded");
    case "duplicate":
      return t("github:dispositionDuplicate");
    case "exploratory":
      return t("github:dispositionExploratory");
    case "withdrawn":
      return t("github:dispositionWithdrawn");
    default:
      return value;
  }
}

/**
 * Records or clears a human-supplied closure reason for a closed, unmerged
 * pull request (AC-31, AC-34: renders nothing for open or merged rows).
 * Selecting `superseded` reveals a URL field; the server's validation error
 * is surfaced verbatim (AC-32). A row that already carries a value shows it
 * and allows it to be changed or cleared (AC-33).
 */
export function PRDispositionRow({ pr }: { pr: TaskPR }) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const setTaskPR = useAppStore((s) => s.setTaskPR);

  const storedDisposition = pr.disposition ?? null;
  const storedURL = pr.disposition_superseded_by_url ?? "";

  const [pendingValue, setPendingValue] = useState<string>(storedDisposition ?? NONE_VALUE);
  const [supersededURL, setSupersededURL] = useState(storedURL);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // A row that already carries a disposition shows it; external updates
  // (WS event from another client, or a reload) must not be clobbered by
  // stale local edit state.
  useEffect(() => {
    setPendingValue(storedDisposition ?? NONE_VALUE);
    setSupersededURL(storedURL);
    setError(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- resync only on the identity of the stored values
  }, [pr.id, storedDisposition, storedURL]);

  if (pr.state !== "closed" || pr.merged_at) return null;

  const save = async (
    disposition: TaskPRDisposition | null,
    supersededByURL: string | null,
  ): Promise<boolean> => {
    if (!workspaceId) return false;
    setSaving(true);
    setError(null);
    try {
      const updated = await patchTaskPRDisposition(pr.id, workspaceId, {
        disposition,
        superseded_by_url: supersededByURL,
      });
      setTaskPR(pr.task_id, updated);
      return true;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("github:dispositionSaveFailed"));
      return false;
    } finally {
      setSaving(false);
    }
  };

  const handleValueChange = (value: string) => {
    setError(null);
    setPendingValue(value);
    if (value === "superseded") return; // wait for the URL + explicit save
    // Auto-save has no separate confirmation step, so a failure must revert
    // the select to the last known-persisted value — otherwise the UI shows
    // the unsaved selection as if it had taken effect, alongside the error.
    void save(value === NONE_VALUE ? null : (value as TaskPRDisposition), null).then((ok) => {
      if (!ok) setPendingValue(storedDisposition ?? NONE_VALUE);
    });
  };

  const showURLField = pendingValue === "superseded";

  return (
    <div data-testid="pr-disposition-row" className="flex flex-col gap-1.5 px-1 py-1.5 text-xs">
      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground">{t("github:dispositionLabel")}</span>
        <Select value={pendingValue} onValueChange={handleValueChange} disabled={saving}>
          <SelectTrigger
            data-testid="pr-disposition-select"
            className="h-7 w-auto min-w-[9rem] text-xs"
          >
            <SelectValue placeholder={t("github:dispositionNone")} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE_VALUE}>{t("github:dispositionNone")}</SelectItem>
            {DISPOSITION_VALUES.map((value) => (
              <SelectItem key={value} value={value}>
                {dispositionLabel(t, value)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {showURLField ? (
        <div className="flex items-center gap-1.5">
          <Input
            data-testid="pr-disposition-superseded-url"
            value={supersededURL}
            onChange={(e) => setSupersededURL(e.target.value)}
            placeholder={t("github:dispositionSupersededByUrlPlaceholder")}
            className="h-7 text-xs"
            disabled={saving}
          />
          <Button
            type="button"
            size="sm"
            variant="secondary"
            className="h-7 cursor-pointer text-xs"
            disabled={saving || supersededURL.trim() === ""}
            onClick={() => void save("superseded", supersededURL.trim())}
            data-testid="pr-disposition-save"
          >
            {t("github:dispositionSave")}
          </Button>
        </div>
      ) : null}
      {error ? (
        <p data-testid="pr-disposition-error" className="text-[11px] leading-snug text-destructive">
          {error}
        </p>
      ) : null}
    </div>
  );
}
