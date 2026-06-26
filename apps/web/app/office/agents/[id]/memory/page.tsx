"use client";

import { use } from "react";
import { AgentMemoryTab } from "../components/agent-memory-tab";
import { useOfficeAgentProfile } from "../use-agent-detail-data";

type Props = { params: Promise<{ id: string }> };

export default function AgentMemoryPage({ params }: Props) {
  const { id } = use(params);
  const agent = useOfficeAgentProfile(id);
  if (!agent) return null;
  return <AgentMemoryTab agent={agent} />;
}
