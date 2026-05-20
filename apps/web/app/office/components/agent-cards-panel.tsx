"use client";

import type { AgentSummary } from "@/lib/state/slices/office/types";
import { AgentCard } from "./agent-card";

type Props = {
  summaries: AgentSummary[];
};

/**
 * Per-agent dashboard cards. Reads its summaries from props (sourced from
 * the dashboard payload's `agent_summaries` field — see Stream A + G of
 * office optimization). The parent `OfficePageClient` owns the dashboard
 * fetch + WS-driven refresh; this component is a pure presenter.
 */
export function AgentCardsPanel({ summaries }: Props) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-3">
      {summaries.map((summary) => (
        <AgentCard key={summary.agent_id} summary={summary} />
      ))}
    </div>
  );
}
