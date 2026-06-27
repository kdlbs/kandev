"use client";

import { use } from "react";
import { AgentPermissionsTab } from "../components/agent-permissions-tab";
import { useOfficeAgentProfile } from "../use-agent-detail-data";

type Props = { params: Promise<{ id: string }> };

export default function AgentPermissionsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useOfficeAgentProfile(id);
  if (!agent) return null;
  return <AgentPermissionsTab agent={agent} />;
}
