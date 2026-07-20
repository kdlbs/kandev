"use client";

import { AutomationsListPage } from "@/components/automations/automations-list-page";

type Props = {
  workspaceId: string;
};

export default function AutomationsPage({ workspaceId }: Props) {
  return <AutomationsListPage workspaceId={workspaceId} />;
}
