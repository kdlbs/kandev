"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { listUnansweredClarificationInbox } from "@/lib/api/domains/clarification-inbox-api";
import { selectNeedsYouInboxRevision } from "@/lib/state/slices/needs-you-inbox/selectors";
import type { AppState } from "@/lib/state/store";
import type { ClarificationInboxUnansweredBundle } from "@/lib/types/clarification-inbox";
import { NeedsYouInboxUnansweredRow } from "./needs-you-inbox-unanswered-row";

type UnansweredStatus = "idle" | "loading" | "ready" | "error";

export function selectActiveWorkspaceUnansweredInboxEnabled(state: AppState): boolean {
  const { items, activeId } = state.workspaces;
  return Boolean(
    items.find((item) => item.id === activeId)?.clarification_unanswered_inbox_enabled,
  );
}

function useUnansweredList(workspaceId: string | null, enabled: boolean, listRevision: number) {
  const [status, setStatus] = useState<UnansweredStatus>("idle");
  const [bundles, setBundles] = useState<ClarificationInboxUnansweredBundle[]>([]);
  const [bundlesWorkspaceId, setBundlesWorkspaceId] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const generationRef = useRef(0);

  const load = useCallback(async () => {
    const generation = ++generationRef.current;
    if (!workspaceId || !enabled) {
      setBundles([]);
      setBundlesWorkspaceId(null);
      setHasMore(false);
      setStatus("idle");
      return;
    }
    setStatus("loading");
    try {
      const page = await listUnansweredClarificationInbox(workspaceId, { cache: "no-store" });
      if (generationRef.current !== generation) return;
      setBundles(page.enabled ? page.bundles : []);
      setBundlesWorkspaceId(workspaceId);
      setHasMore(Boolean(page.enabled && page.next_cursor));
      setStatus("ready");
    } catch {
      if (generationRef.current !== generation) return;
      setStatus("error");
    }
  }, [workspaceId, enabled]);

  // Every Needs-you refresh (WS events, focus, the periodic tick, an answer
  // elsewhere) re-reads this section too, so both lists move together.
  useEffect(() => {
    void load();
  }, [load, listRevision]);

  // Only the current workspace's rows are ever shown. The active workspace can
  // change between a fetch and its result, so a bundle is rendered only while
  // the workspace it was read for is still the active one; otherwise a switch
  // would briefly surface the previous workspace's questions and their actions.
  const scoped = bundlesWorkspaceId === workspaceId;
  return { status, bundles: scoped ? bundles : [], hasMore: scoped && hasMore, load };
}

/**
 * The Needs-you Inbox's "unanswered" section: questions the agent stopped
 * waiting on after a newer turn. Rendered only while the workspace setting is
 * on; the answerable list above is unchanged by it.
 */
export function NeedsYouInboxUnansweredSection() {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const enabled = useAppStore(selectActiveWorkspaceUnansweredInboxEnabled);
  const listRevision = useAppStore(selectNeedsYouInboxRevision);
  const { status, bundles, hasMore, load } = useUnansweredList(workspaceId, enabled, listRevision);
  const reload = useCallback(() => void load(), [load]);

  if (!enabled || status === "idle") return null;
  if (status === "ready" && bundles.length === 0) return null;

  return (
    <section
      className="space-y-2"
      aria-labelledby="needs-you-unanswered-heading"
      data-testid="needs-you-unanswered-section"
    >
      <div className="space-y-0.5">
        <h2 id="needs-you-unanswered-heading" className="text-sm font-medium">
          {t("needsYouInbox:unansweredTitle")}
        </h2>
        <p className="text-xs text-muted-foreground">{t("needsYouInbox:unansweredDescription")}</p>
      </div>
      {status === "loading" && bundles.length === 0 && (
        <p className="text-xs text-muted-foreground" role="status">
          {t("common:loading")}
        </p>
      )}
      {status === "error" && (
        <div className="flex items-center justify-between gap-2 rounded-lg border border-border p-3">
          <p className="text-xs text-destructive">{t("needsYouInbox:unansweredLoadFailed")}</p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={reload}
          >
            {t("needsYouInbox:retry")}
          </Button>
        </div>
      )}
      {bundles.length > 0 && (
        <div className="overflow-hidden rounded-lg border border-dashed border-border divide-y divide-border">
          {bundles.map((bundle) => (
            <NeedsYouInboxUnansweredRow
              key={bundle.pending_id}
              bundle={bundle}
              onChanged={reload}
            />
          ))}
        </div>
      )}
      {hasMore && (
        <p className="text-xs text-muted-foreground">{t("needsYouInbox:truncatedNotice")}</p>
      )}
    </section>
  );
}
