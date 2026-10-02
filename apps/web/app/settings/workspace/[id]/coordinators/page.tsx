"use client";

import { CoordinatorsListPage } from "@/components/coordinators/coordinators-list-page";

type Props = {
  workspaceId: string;
};

export default function CoordinatorsPage({ workspaceId }: Props) {
  return <CoordinatorsListPage workspaceId={workspaceId} />;
}
