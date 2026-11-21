"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { NoteVersion } from "@/lib/api/domains/coordinator-notes-api";
import { citationSummary, formatNoteTime, noteTypeLabel } from "@/lib/coordinator/notes";

type Props = {
  versions: NoteVersion[];
  currentVersion: number;
  canRevert: boolean;
  busy: boolean;
  onRevert: (version: number) => void;
};

function provenanceText(t: ReturnType<typeof useTranslation>["t"], v: NoteVersion): string {
  if (v.reverted_from)
    return t("coordinator:noteVersionRevertedFrom", { version: v.reverted_from });
  return v.provenance.kind === "model"
    ? t("coordinator:noteProvenanceModel")
    : t("coordinator:noteProvenanceHuman");
}

/** Every stored version, newest first; the current one cannot be reverted to. */
export function NoteVersions({ versions, currentVersion, canRevert, busy, onRevert }: Props) {
  const { t } = useTranslation();
  const ordered = [...versions].sort((a, b) => b.version - a.version);
  return (
    <section className="space-y-2" data-testid="note-versions">
      <h4 className="text-sm font-medium">{t("coordinator:noteVersionsHeading")}</h4>
      <ul className="space-y-2">
        {ordered.map((v) => (
          <li
            key={v.version}
            className="space-y-1 rounded-md border p-2 text-sm"
            data-testid={`note-version-${v.version}`}
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <span className="font-medium">
                {t("coordinator:noteVersionNumber", { version: v.version })}
              </span>
              <span className="text-xs text-muted-foreground">
                {formatNoteTime(v.created_at)} · {provenanceText(t, v)}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">{noteTypeLabel(t, v.type)}</p>
            <p className="whitespace-pre-wrap break-words">{v.text}</p>
            <ul className="text-xs text-muted-foreground">
              {v.citations.map((c, i) => (
                <li key={i} className="break-all">
                  {citationSummary(c)}
                </li>
              ))}
            </ul>
            {canRevert && v.version !== currentVersion && (
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                className="min-h-11 cursor-pointer sm:min-h-8"
                data-testid={`note-revert-${v.version}`}
                onClick={() => onRevert(v.version)}
              >
                {t("coordinator:noteRevert")}
              </Button>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
