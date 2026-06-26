"use client";

import { use } from "react";
import { AgentConfigurationTab } from "../components/agent-configuration-tab";
import { useOfficeAgentProfile } from "../use-agent-detail-data";

type Props = { params: Promise<{ id: string }> };

export default function AgentConfigurationPage({ params }: Props) {
  const { id } = use(params);
  const agent = useOfficeAgentProfile(id);
  if (!agent) return null;
  return <AgentConfigurationTab agent={agent} />;
}
