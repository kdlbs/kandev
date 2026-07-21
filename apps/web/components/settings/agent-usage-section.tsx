"use client";

import { IconAlertTriangle, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Separator } from "@kandev/ui/separator";
import { cn } from "@/lib/utils";
import { AgentLogo } from "@/components/agent-logo";
import { UsageWindowRows, usageStatus } from "@/components/usage/usage-window-rows";
import { useAgentSubscriptionUsage } from "@/hooks/domains/settings/use-agent-subscription-usage";
import type { AgentSubscriptionUsage } from "@/lib/types/http";

function AgentUsageCard({ item }: { item: AgentSubscriptionUsage }) {
  const usage = item.usage;
  const status = usage && usage.windows.length > 0 ? usageStatus(usage) : null;
  return (
    <Card data-testid={`agent-usage-card-${item.agent_id}`}>
      <CardContent className="py-4 space-y-4">
        <div className="flex items-center gap-2">
          <AgentLogo agentName={item.agent_id} className="shrink-0" />
          <h4 className="font-medium">{item.display_name}</h4>
          {usage?.plan && (
            <Badge variant="secondary" className="capitalize">
              {usage.plan}
            </Badge>
          )}
          {status && (
            <span className={cn("ml-auto text-xs font-semibold", status.className)}>
              {status.label}
            </span>
          )}
        </div>
        {item.error ? (
          <p className="flex items-center gap-1 text-xs text-muted-foreground">
            <IconAlertTriangle className="h-3.5 w-3.5 shrink-0" />
            Could not fetch usage data.
          </p>
        ) : (
          usage && <UsageWindowRows usage={usage} />
        )}
      </CardContent>
    </Card>
  );
}

/**
 * "Subscription Usage" section on Settings > Agents. Shows rate-limit window
 * utilization for host agents authenticated with a subscription (OAuth) —
 * Claude Code and Codex. Hidden when no subscription-billed agent is present.
 */
export function AgentUsageSection() {
  const { items, loading, refresh } = useAgentSubscriptionUsage();
  if (items.length === 0) return null;

  return (
    <div className="space-y-4" data-testid="agent-usage-section">
      <Separator />
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-lg font-semibold">Subscription Usage</h3>
          <p className="text-sm text-muted-foreground">
            Rate-limit utilization for agents signed in with a subscription plan.
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void refresh()}
          disabled={loading}
          className="cursor-pointer"
        >
          {loading ? (
            <IconLoader2 className="h-4 w-4 mr-2 animate-spin" />
          ) : (
            <IconRefresh className="h-4 w-4 mr-2" />
          )}
          Refresh
        </Button>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {items.map((item) => (
          <AgentUsageCard key={item.agent_id} item={item} />
        ))}
      </div>
    </div>
  );
}
