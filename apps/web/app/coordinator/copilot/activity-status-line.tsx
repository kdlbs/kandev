"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { formatActivityDuration, type ActivityStatusLine } from "./activity-display";

/** The live line above the composer while a turn runs. Only a change of verb is
 *  announced: the elapsed seconds sit in an aria-hidden span. */
export function ActivityStatusLineView({ line }: { line: ActivityStatusLine }) {
  const { t } = useTranslation();
  const firstSight = useRef(Date.now());
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  const started = line.startedAtMs ?? firstSight.current;
  const seconds = Math.max(0, Math.floor((now - started) / 1000));
  return (
    <div
      role="status"
      aria-live="polite"
      data-testid="activity-status-line"
      className="flex items-center gap-2 px-3 py-1.5 text-xs text-muted-foreground"
    >
      <span className="inline-block h-1.5 w-1.5 rounded-full bg-primary motion-safe:animate-pulse" />
      <span>{t(line.verbKey)}</span>
      <span aria-hidden="true" className="tabular-nums">
        {formatActivityDuration(seconds)}
      </span>
    </div>
  );
}

/** Renders the line for the running turn, remounting per turn so the
 *  first-sight fallback restarts. */
export function ActivityStatusSlot({ line }: { line: ActivityStatusLine | null }) {
  if (!line) return null;
  return <ActivityStatusLineView key={line.turnId ?? "no-turn"} line={line} />;
}
