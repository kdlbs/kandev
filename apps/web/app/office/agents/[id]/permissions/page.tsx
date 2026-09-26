"use client";

import { use } from "react";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfile } from "@/lib/state/slices/office/selectors";
import { AgentPermissionsTab } from "../components/agent-permissions-tab";

type Props = { params: Promise<{ id: string }> };

export default function AgentPermissionsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useAppStore((s) => selectOfficeAgentProfile(s, id));
  if (!agent) return null;
  return <AgentPermissionsTab agent={agent} />;
}
