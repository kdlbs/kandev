"use client";

import { useTranslation } from "react-i18next";
import { CardDescription } from "@kandev/ui/card";
import { useProposalById } from "@/hooks/domains/coordinator/use-proposals";

/** "Revised after your reply" with the quoted condition, when the original row is readable. */
export function RevisedNote({
  workspaceId,
  coordinatorId,
  inReplyTo,
}: {
  workspaceId: string;
  coordinatorId: string;
  inReplyTo: string;
}) {
  const { t } = useTranslation();
  const { proposal } = useProposalById(workspaceId, coordinatorId, inReplyTo);
  const quote = proposal?.status === "returned" ? proposal.reply_text : null;
  return (
    <div data-testid="proposal-revised-note">
      <CardDescription>{t("coordinator:revisedAfterReply")}</CardDescription>
      {quote && <CardDescription>{t("coordinator:revisedQuote", { text: quote })}</CardDescription>}
    </div>
  );
}
