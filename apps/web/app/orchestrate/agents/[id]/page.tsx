"use client";

import { use } from "react";
import { AgentDetailContent } from "./components/agent-detail-content";

type AgentDetailPageProps = {
  params: Promise<{ id: string }>;
};

export default function AgentDetailPage({ params }: AgentDetailPageProps) {
  const { id } = use(params);
  return <AgentDetailContent agentId={id} />;
}
