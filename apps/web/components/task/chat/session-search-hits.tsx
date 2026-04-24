"use client";

import { useMemo } from "react";
import { cn } from "@/lib/utils";
import { AgentLogo } from "@/components/agent-logo";
import type { MessageSearchHit } from "@/lib/api/domains/session-api";

type SessionSearchHitsProps = {
  hits: MessageSearchHit[];
  query: string;
  activeHitId: string | null;
  onSelect: (id: string) => void;
  isSearching: boolean;
  /** Display name for agent hits (e.g. the profile name). Falls back to "Agent". */
  agentLabel?: string | null;
  /** Agent registry slug (e.g. "claude-code") used to fetch the profile logo. */
  agentName?: string | null;
};

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

function HighlightedSnippet({ text, query }: { text: string; query: string }) {
  const parts = useMemo(() => {
    const q = query.trim();
    if (!q) return [{ text, match: false }];
    const escaped = q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const re = new RegExp(escaped, "gi");
    const result: Array<{ text: string; match: boolean }> = [];
    let last = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
      if (m.index > last) result.push({ text: text.slice(last, m.index), match: false });
      result.push({ text: m[0], match: true });
      last = m.index + m[0].length;
      if (m.index === re.lastIndex) re.lastIndex++;
    }
    if (last < text.length) result.push({ text: text.slice(last), match: false });
    return result;
  }, [text, query]);

  return (
    <span>
      {parts.map((p, i) =>
        p.match ? (
          <mark
            key={i}
            className="bg-yellow-200/80 dark:bg-yellow-500/40 text-foreground rounded-sm px-0.5"
          >
            {p.text}
          </mark>
        ) : (
          <span key={i}>{p.text}</span>
        ),
      )}
    </span>
  );
}

function HitAuthor({
  authorType,
  agentLabel,
  agentName,
}: {
  authorType: string;
  agentLabel?: string | null;
  agentName?: string | null;
}) {
  if (authorType === "user") {
    return (
      <span className="text-[0.6875rem] uppercase tracking-wide font-medium text-primary/80 truncate">
        You
      </span>
    );
  }
  const label = agentLabel?.trim() || "Agent";
  return (
    <span className="text-[0.6875rem] uppercase tracking-wide font-medium text-muted-foreground inline-flex items-center gap-1.5 min-w-0">
      {agentName && <AgentLogo agentName={agentName} size={12} className="shrink-0" />}
      <span className="truncate">{label}</span>
    </span>
  );
}

export function SessionSearchHits({
  hits,
  query,
  activeHitId,
  onSelect,
  isSearching,
  agentLabel,
  agentName,
}: SessionSearchHitsProps) {
  if (!query.trim()) return null;
  return (
    <div className="w-[28rem] max-h-80 overflow-auto rounded-md border border-border bg-background shadow-lg text-xs">
      {isSearching && hits.length === 0 && (
        <div className="p-3 text-muted-foreground">Searching…</div>
      )}
      {!isSearching && hits.length === 0 && (
        <div className="p-3 text-muted-foreground">No matches</div>
      )}
      {hits.map((hit) => (
        <button
          key={hit.id}
          type="button"
          onClick={() => onSelect(hit.id)}
          className={cn(
            "w-full text-left px-3 py-2 border-b border-border last:border-0 cursor-pointer hover:bg-muted/50",
            activeHitId === hit.id && "bg-muted",
          )}
        >
          <div className="flex items-center justify-between gap-2 mb-0.5">
            <HitAuthor authorType={hit.author_type} agentLabel={agentLabel} agentName={agentName} />
            <span className="text-[0.6875rem] text-muted-foreground/70 shrink-0">
              {formatTime(hit.created_at)}
            </span>
          </div>
          <div className="text-foreground/90 line-clamp-3 whitespace-pre-wrap break-words">
            <HighlightedSnippet text={hit.snippet} query={query} />
          </div>
        </button>
      ))}
    </div>
  );
}
