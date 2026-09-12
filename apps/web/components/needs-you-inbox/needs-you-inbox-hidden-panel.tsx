"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { toast } from "@/lib/toast/sonner";
import { formatRelativeTime } from "@/lib/i18n/formats";
import {
  listHiddenClarificationInbox,
  restoreClarificationInboxBundle,
} from "@/lib/api/domains/clarification-inbox-api";
import type { ClarificationInboxHiddenBundle } from "@/lib/types/clarification-inbox";
import { rowPrimaryText } from "@/lib/needs-you-inbox/row-presentation";

type HiddenListStatus = "idle" | "loading" | "ready" | "error";

function HiddenRow({
  bundle,
  onRestore,
}: {
  bundle: ClarificationInboxHiddenBundle;
  onRestore: (pendingId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-center justify-between gap-2 rounded-md border border-border px-3 py-2"
      data-testid="needs-you-inbox-hidden-row"
    >
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">
          {rowPrimaryText(bundle, t("needsYouInbox:questionFromAgent"))}
        </p>
        <p className="text-xs text-muted-foreground">
          {bundle.state === "snoozed" && bundle.snooze_until
            ? t("needsYouInbox:snoozedUntil", { time: formatRelativeTime(bundle.snooze_until) })
            : t("needsYouInbox:dismissed")}
        </p>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="cursor-pointer shrink-0"
        onClick={() => onRestore(bundle.pending_id)}
      >
        {t("needsYouInbox:restore")}
      </Button>
    </div>
  );
}

function useHiddenList(workspaceId: string | null) {
  const [status, setStatus] = useState<HiddenListStatus>("idle");
  const [bundles, setBundles] = useState<ClarificationInboxHiddenBundle[]>([]);

  const load = useCallback(async () => {
    if (!workspaceId) return;
    setStatus("loading");
    try {
      const page = await listHiddenClarificationInbox(workspaceId);
      setBundles(page.bundles);
      setStatus("ready");
    } catch {
      setStatus("error");
    }
  }, [workspaceId]);

  return { status, bundles, load };
}

// AC .33/.37: discloses how many answerable bundles this operator's own
// dismiss or snooze is hiding, and lets them enumerate and restore one.
export function NeedsYouInboxHiddenPanel({ hiddenCount }: { hiddenCount: number }) {
  const { t } = useTranslation();
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const bumpRefreshTick = useAppStore((s) => s.bumpNeedsYouInboxRefreshTick);
  const [open, setOpen] = useState(false);
  const { status, bundles, load } = useHiddenList(workspaceId);

  const toggle = useCallback(() => {
    setOpen((current) => {
      const next = !current;
      if (next) void load();
      return next;
    });
  }, [load]);

  const handleRestore = useCallback(
    async (pendingId: string) => {
      try {
        await restoreClarificationInboxBundle(pendingId);
        bumpRefreshTick();
        void load();
      } catch {
        toast.error(t("needsYouInbox:restoreFailed"));
      }
    },
    [bumpRefreshTick, load, t],
  );

  return (
    <div className="rounded-lg border border-border p-4" data-testid="needs-you-inbox-hidden-panel">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {t("needsYouInbox:hiddenNotice", { count: hiddenCount })}
        </p>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={toggle}
        >
          {open ? t("needsYouInbox:hideHidden") : t("needsYouInbox:showHidden")}
        </Button>
      </div>
      {open && (
        <div className="mt-3 space-y-2">
          {status === "loading" && (
            <p className="text-xs text-muted-foreground">{t("needsYouInbox:hiddenLoading")}</p>
          )}
          {status === "error" && (
            <p className="text-xs text-destructive">{t("needsYouInbox:hiddenLoadFailed")}</p>
          )}
          {status === "ready" && bundles.length === 0 && (
            <p className="text-xs text-muted-foreground">{t("needsYouInbox:hiddenEmpty")}</p>
          )}
          {status === "ready" &&
            bundles.map((bundle) => (
              <HiddenRow key={bundle.pending_id} bundle={bundle} onRestore={handleRestore} />
            ))}
        </div>
      )}
    </div>
  );
}
