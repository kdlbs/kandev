"use client";

import { use } from "react";
import { useAppStore } from "@/components/state-provider";
import { AgentConfigurationTab } from "../components/agent-configuration-tab";

type Props = { params: Promise<{ id: string }> };

export default function AgentConfigurationPage({ params }: Props) {
  const { id } = use(params);
  const agent = useAppStore((s) => s.office.agentProfiles.find((a) => a.id === id));
  if (!agent) return null;
  return <AgentConfigurationTab agent={agent} />;
}
