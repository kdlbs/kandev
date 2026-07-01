"use client";

import { use } from "react";
import { AgentChannelsTab } from "../components/agent-channels-tab";
import { useOfficeAgentProfile } from "../use-agent-detail-data";

type Props = { params: Promise<{ id: string }> };

export default function AgentChannelsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useOfficeAgentProfile(id);
  if (!agent) return null;
  return <AgentChannelsTab agent={agent} />;
}
