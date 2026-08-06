"use client";

import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent } from "@kandev/ui/card";
import { Switch } from "@kandev/ui/switch";
import { AgentLogo } from "@/components/agent-logo";
import type { Agent, AgentProfile } from "@/lib/types/http";

type ProfileListItemProps = {
  agent: Agent;
  profile: AgentProfile;
  onToggleEnabled: (profile: AgentProfile, enabled: boolean) => void;
};

export function ProfileListItem({ agent, profile, onToggleEnabled }: ProfileListItemProps) {
  const { t } = useTranslation();
  const profilePath = `/settings/agents/${encodeURIComponent(agent.name)}/profiles/${profile.id}`;
  const enabled = profile.enabled ?? true;
  return (
    <Card className="hover:bg-accent transition-colors">
      <CardContent className="py-2 flex items-center justify-between gap-3">
        <Link href={profilePath} className="flex items-center gap-2 min-w-0 flex-1 cursor-pointer">
          <AgentLogo agentName={agent.name} className="shrink-0" />
          <span className="text-sm font-medium">
            {agent.profiles[0]?.agentDisplayName ?? agent.name}
          </span>
          {agent.supports_mcp && <Badge variant="secondary">MCP</Badge>}
          <span className="text-sm text-muted-foreground truncate">{profile.name}</span>
        </Link>
        {/* Outside the Link on purpose: a click on a switch nested inside an
            anchor still follows the href as the native default action (only
            preventDefault cancels it, and Radix skips its toggle when the
            consumer calls preventDefault), and interactive-inside-interactive
            is an a11y anti-pattern. As a sibling, toggling never navigates. */}
        <Switch
          checked={enabled}
          onCheckedChange={(next) => onToggleEnabled(profile, next)}
          data-testid={`profile-enabled-toggle-${profile.id}`}
          aria-label={
            enabled
              ? t("agents:disableProfileNamed", { name: profile.name })
              : t("agents:enableProfileNamed", { name: profile.name })
          }
        />
      </CardContent>
    </Card>
  );
}
