"use client";

import { useState } from "react";
import { IconExternalLink, IconTrash } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";

import { revokeShare, type Share } from "@/lib/api/domains/share-api";

type Props = {
  shares: Share[];
  onRevoked: () => void;
};

/**
 * Renders the list of active (non-revoked) shares with a per-row revoke
 * button. Revoke is optimistic: the API call fires; on success we call
 * onRevoked() so the parent can refresh from the source of truth.
 */
export function ShareList({ shares, onRevoked }: Props) {
  const active = shares.filter((s) => !s.revoked_at);
  if (active.length === 0) return null;

  return (
    <div className="flex flex-col gap-2 rounded border bg-muted/30 p-3">
      <div className="text-xs font-medium text-muted-foreground">
        Active shares for this session
      </div>
      <ul className="flex flex-col gap-1.5">
        {active.map((share) => (
          <ShareRow key={share.id} share={share} onRevoked={onRevoked} />
        ))}
      </ul>
    </div>
  );
}

function ShareRow({ share, onRevoked }: { share: Share; onRevoked: () => void }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const handleRevoke = async () => {
    setBusy(true);
    setErr(null);
    try {
      await revokeShare(share.id);
      onRevoked();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to revoke share.");
    } finally {
      // Always clear busy — on success the row usually unmounts via the
      // parent's refresh, but if it stays rendered (e.g. error path or a
      // not-yet-reflected list) we don't want it stuck in "Revoking…".
      setBusy(false);
    }
  };

  return (
    <li className="flex min-w-0 flex-col gap-1">
      <div className="flex min-w-0 items-center justify-between gap-2 text-sm">
        <a
          href={share.url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex min-w-0 flex-1 items-center gap-1 text-primary hover:underline cursor-pointer"
        >
          <IconExternalLink className="h-3 w-3 flex-shrink-0" />
          <span className="min-w-0 flex-1 truncate" title={share.url}>
            {share.url}
          </span>
        </a>
        <Button
          size="sm"
          variant="ghost"
          onClick={handleRevoke}
          disabled={busy}
          className="flex-shrink-0 cursor-pointer"
          aria-label="Revoke share"
        >
          <IconTrash className="h-3 w-3" />
          {busy ? "Revoking…" : "Revoke"}
        </Button>
      </div>
      {err && <span className="text-xs text-destructive">{err}</span>}
    </li>
  );
}
