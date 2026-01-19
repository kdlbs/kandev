'use client';

import Link from 'next/link';
import { IconRobot, IconChevronRight } from '@tabler/icons-react';
import { Card, CardContent } from '@kandev/ui/card';
import { Badge } from '@kandev/ui/badge';
import type { AgentProfile } from '@/lib/settings/types';

type AgentCardProps = {
  agent: AgentProfile;
};

export function AgentCard({ agent }: AgentCardProps) {
  const agentLabel = agent.agentDisplayName;

  return (
    <Link href={`/settings/agents/${agent.id}`}>
      <Card className="hover:bg-accent transition-colors cursor-pointer">
        <CardContent className="py-4">
          <div className="flex items-start justify-between">
            <div className="flex items-start gap-3 flex-1">
              <div className="p-2 bg-muted rounded-md">
                <IconRobot className="h-4 w-4" />
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                  <h4 className="font-medium">{agent.name}</h4>
                  <Badge variant="secondary" className="text-xs">
                    {agentLabel}
                  </Badge>
                  {agent.autoApprove && (
                    <Badge variant="outline" className="text-xs text-green-600">
                      Auto-approve
                    </Badge>
                  )}
                </div>
                <div className="text-sm text-muted-foreground mt-1">
                  <p>Model: {agent.model}</p>
                  <p>Temperature: {agent.temperature}</p>
                </div>
              </div>
            </div>
            <IconChevronRight className="h-5 w-5 text-muted-foreground" />
          </div>
        </CardContent>
      </Card>
    </Link>
  );
}
