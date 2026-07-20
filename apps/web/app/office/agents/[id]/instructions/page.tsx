"use client";

import { use } from "react";
import { AgentInstructionsTab } from "../components/agent-instructions-tab";
import { useOfficeAgentProfile } from "../use-agent-detail-data";

type Props = { params: Promise<{ id: string }> };

export default function AgentInstructionsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useOfficeAgentProfile(id);
  if (!agent) return null;
  return <AgentInstructionsTab agent={agent} />;
}
