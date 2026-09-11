"use client";

import { CoordinatorGrantsPage } from "@/components/settings/workspaces/coordinators/coordinator-grants-page";

type Props = {
  workspaceId: string;
};

export default function CoordinatorsPage({ workspaceId }: Props) {
  return <CoordinatorGrantsPage workspaceId={workspaceId} />;
}
