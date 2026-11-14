"use client";

import { useTranslation } from "react-i18next";
import type { NoteReference, NoteReferrers } from "@/lib/api/domains/coordinator-notes-api";

type Props = { referrers?: NoteReferrers; history: NoteReference[] };

/** Open proposals and tasks that cite the note now, then every proposal that ever did. */
export function NoteReferences({ referrers, history }: Props) {
  const { t } = useTranslation();
  const proposals = referrers?.proposals ?? [];
  const tasks = referrers?.tasks ?? [];
  return (
    <section className="space-y-2" data-testid="note-references">
      <h4 className="text-sm font-medium">{t("coordinator:noteReferencedByHeading")}</h4>
      {proposals.length === 0 && tasks.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("coordinator:noteReferencedByNone")}</p>
      ) : (
        <ul className="text-sm">
          {proposals.map((p) => (
            <li key={`p-${p.id}`} className="break-all">
              {t("coordinator:noteReferencedByProposal", { id: p.id, status: p.status })}
            </li>
          ))}
          {tasks.map((task) => (
            <li key={`t-${task.id}`} className="break-all">
              {t("coordinator:noteReferencedByTask", { id: task.id })}
            </li>
          ))}
        </ul>
      )}
      <h4 className="text-sm font-medium">{t("coordinator:noteHistoryHeading")}</h4>
      {history.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("coordinator:noteHistoryNone")}</p>
      ) : (
        <ul className="text-sm" data-testid="note-history">
          {history.map((h) => (
            <li key={`${h.proposal_id}-${h.note_version}`} className="break-all">
              {t("coordinator:noteHistoryRow", {
                id: h.proposal_id,
                status: h.status,
                version: h.note_version,
              })}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
