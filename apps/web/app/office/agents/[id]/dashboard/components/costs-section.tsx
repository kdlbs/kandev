import Link from "next/link";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import type { AgentCostAggregate, AgentRunCost } from "@/lib/api/domains/office-extended-api";
import { formatSubcents, formatShortDate } from "./format-date";

type Props = {
  agentId: string;
  aggregate: AgentCostAggregate;
  recent: AgentRunCost[];
};

/**
 * Compact integer format for token columns. SI suffixes for >999 so
 * the table stays narrow without losing meaning. Falls back to the
 * raw integer for smaller values.
 */
function formatTokens(n: number): string {
  if (!Number.isFinite(n) || n === 0) return "0";
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return String(n);
}

export function CostsSection({ agentId, aggregate, recent }: Props) {
  return (
    <Card data-testid="costs-section">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Costs</CardTitle>
      </CardHeader>
      <CardContent className="pt-0 space-y-4">
        <CostAggregateRow aggregate={aggregate} />
        <RecentRunCostsTable agentId={agentId} recent={recent} />
      </CardContent>
    </Card>
  );
}

function CostAggregateRow({ aggregate }: { aggregate: AgentCostAggregate }) {
  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 text-sm" data-testid="cost-aggregate">
      <Stat label="Input tokens" value={formatTokens(aggregate.input_tokens)} testId="agg-input" />
      <Stat
        label="Output tokens"
        value={formatTokens(aggregate.output_tokens)}
        testId="agg-output"
      />
      <Stat
        label="Cached tokens"
        value={formatTokens(aggregate.cached_tokens)}
        testId="agg-cached"
      />
      <Stat
        label="Total cost"
        value={formatSubcents(aggregate.total_cost_subcents)}
        testId="agg-total-cost"
      />
    </div>
  );
}

function Stat({ label, value, testId }: { label: string; value: string; testId: string }) {
  return (
    <div className="space-y-0.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="font-medium" data-testid={testId}>
        {value}
      </div>
    </div>
  );
}

function RecentRunCostsTable({ agentId, recent }: { agentId: string; recent: AgentRunCost[] }) {
  if (recent.length === 0) {
    return (
      <p className="text-sm text-muted-foreground" data-testid="cost-recent-empty">
        No runs with cost data yet.
      </p>
    );
  }
  return (
    <Table data-testid="recent-run-costs-table">
      <TableHeader>
        <TableRow>
          <TableHead>Date</TableHead>
          <TableHead>Run</TableHead>
          <TableHead className="text-right">Input</TableHead>
          <TableHead className="text-right">Output</TableHead>
          <TableHead className="text-right">Cost</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {recent.map((row) => (
          <TableRow key={row.run_id} data-testid="recent-run-cost-row">
            <TableCell className="text-xs text-muted-foreground">
              {formatShortDate(row.date)}
            </TableCell>
            <TableCell>
              <Link
                href={`/office/agents/${agentId}/runs/${row.run_id}`}
                className="text-xs font-mono hover:underline cursor-pointer"
              >
                {row.run_id_short}
              </Link>
            </TableCell>
            <TableCell className="text-right text-xs">{formatTokens(row.input_tokens)}</TableCell>
            <TableCell className="text-right text-xs">{formatTokens(row.output_tokens)}</TableCell>
            <TableCell className="text-right text-xs font-medium">
              {formatSubcents(row.cost_subcents)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
