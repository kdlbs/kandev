"use client";

import { IconPlus, IconRoute } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { DisabledBadge } from "@/components/settings/record-badges";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { DYNAMIC_AGENT_NAME } from "@/lib/settings/agent-display-order";
import type { Agent, AgentProfile } from "@/lib/types/http";

const dynamicProfileRoute = (profileId: string): string =>
  `/settings/agents/${encodeURIComponent(DYNAMIC_AGENT_NAME)}/profiles/${encodeURIComponent(profileId)}`;

function DynamicProfileRow({ profile }: { profile: AgentProfile }) {
  const { t } = useTranslation();
  const candidateCount = profile.dynamic?.candidates.length ?? 0;

  return (
    <Link
      href={dynamicProfileRoute(profile.id)}
      className="flex min-h-11 min-w-0 items-center justify-between gap-3 rounded-md border p-3 transition-colors hover:border-foreground/30 hover:bg-muted/50 cursor-pointer"
      data-testid={`dynamic-profile-route-${profile.id}`}
    >
      <div className="flex min-w-0 items-center gap-2">
        <span aria-hidden className="h-1.5 w-1.5 shrink-0 rounded-full bg-primary/70" />
        <span className="min-w-0 truncate text-sm font-medium">
          {profile.name || t("agents:profileName")}
        </span>
        {profile.enabled === false && <DisabledBadge />}
      </div>
      <Badge variant="outline" className="shrink-0">
        {t("agents:dynamicCandidates")}: {candidateCount}
      </Badge>
    </Link>
  );
}

type DynamicAgentsCardProps = {
  agent?: Pick<Agent, "profiles">;
};

export function DynamicAgentsCard({ agent }: DynamicAgentsCardProps) {
  const { t } = useTranslation();
  const routingEnabled = useFeature("dynamicAgentRouting");

  if (!routingEnabled) return null;

  const profiles = agent?.profiles ?? [];

  return (
    <Card className="min-w-0 gap-0 py-0" data-testid="dynamic-agents-card">
      <CardHeader className="flex flex-col gap-3 px-3 py-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <CardTitle className="flex items-center gap-2 text-lg">
            <IconRoute className="h-5 w-5 shrink-0" aria-hidden />
            {t("agents:dynamicAgents")}
          </CardTitle>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("agents:dynamicAgentsDescription")}
          </p>
        </div>
        <Button
          size="sm"
          className="min-h-11 cursor-pointer sm:min-h-7"
          asChild
          data-testid="new-dynamic-profile"
        >
          <Link href={`/settings/agents/${DYNAMIC_AGENT_NAME}?mode=create`}>
            <IconPlus className="mr-2 h-4 w-4" />
            {t("agents:newProfile")}
          </Link>
        </Button>
      </CardHeader>
      <CardContent className="space-y-2 px-3 pb-3">
        {profiles.length > 0 ? (
          profiles.map((profile) => <DynamicProfileRow key={profile.id} profile={profile} />)
        ) : (
          <p
            className="rounded-md border border-dashed p-4 text-sm text-muted-foreground"
            data-testid="dynamic-agents-empty"
          >
            {t("agents:noProfilesYetShort")}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
