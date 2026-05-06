"use client";

import { use } from "react";
import { useAppStore } from "@/components/state-provider";
import { AgentChannelsTab } from "../components/agent-channels-tab";

type Props = { params: Promise<{ id: string }> };

export default function AgentChannelsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useAppStore((s) => s.office.agentProfiles.find((a) => a.id === id));
  if (!agent) return null;
  return <AgentChannelsTab agent={agent} />;
}
