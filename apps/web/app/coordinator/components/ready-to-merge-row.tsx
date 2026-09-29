"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { getPrimaryTaskPR } from "@/hooks/domains/github/use-task-pr";
import type { QueueItem } from "@/lib/coordinator/attention";
import type { TaskPR } from "@/lib/types/github";
import { SendBackForm, sessionAcceptsMessage } from "./send-back-form";

/** The PR's URL when it is a plain http(s) link, else undefined. */
export function safePullRequestUrl(url: string | undefined): string | undefined {
  if (!url) return undefined;
  try {
    const parsed = new URL(url);
    return parsed.protocol === "https:" || parsed.protocol === "http:" ? url : undefined;
  } catch {
    return undefined;
  }
}

type ReadyToMergeActionsProps = {
  item: QueueItem;
  prsByTaskId: ReadonlyMap<string, TaskPR[]>;
  canManage: boolean;
  children: (link: React.ReactNode) => React.ReactNode;
};

/**
 * The Ready to merge row's actions, kept outside the row's task link so
 * pressing one never navigates. `children` receives the actions cell and
 * renders it after the link.
 */
export function ReadyToMergeActions({
  item,
  prsByTaskId,
  canManage,
  children,
}: ReadyToMergeActionsProps) {
  const { t } = useTranslation();
  const [formOpen, setFormOpen] = useState(false);
  const openButtonRef = useRef<HTMLButtonElement>(null);
  const task = item.task;
  const prUrl = safePullRequestUrl(getPrimaryTaskPR(prsByTaskId.get(task.id))?.pr_url);
  const session = task.statusSummary?.primary_session;
  const canSendBack = canManage && !!session?.id && sessionAcceptsMessage(session.state);
  const label = task.identifier ?? task.id;

  useEffect(() => {
    if (!canSendBack) setFormOpen(false);
  }, [canSendBack]);

  function closeForm() {
    setFormOpen(false);
    requestAnimationFrame(() => openButtonRef.current?.focus());
  }

  const actions = (
    <span className="flex flex-wrap items-center gap-2">
      {prUrl && (
        <Button asChild variant="outline" size="sm" className="cursor-pointer">
          <a href={prUrl} target="_blank" rel="noopener noreferrer">
            {t("coordinator:openPr")}
          </a>
        </Button>
      )}
      {canSendBack && (
        <>
          <span className="text-muted-foreground text-xs">{t("coordinator:sendBackPrompt")}</span>
          <Button
            ref={openButtonRef}
            variant="outline"
            size="sm"
            className="cursor-pointer"
            aria-expanded={formOpen}
            onClick={() => setFormOpen((open) => !open)}
          >
            {t("coordinator:sendBackAction")}
          </Button>
        </>
      )}
    </span>
  );

  return (
    <div data-testid={`queue-ready-row-${task.id}`}>
      {children(actions)}
      {canSendBack && formOpen && session?.id && (
        <SendBackForm taskId={task.id} sessionId={session.id} label={label} onClose={closeForm} />
      )}
    </div>
  );
}
