"use client";

import { CoordinatorAddPage } from "@/components/coordinators/coordinator-add-page";

type Props = {
  workspaceId: string;
};

export default function NewCoordinatorPage({ workspaceId }: Props) {
  return <CoordinatorAddPage workspaceId={workspaceId} />;
}
