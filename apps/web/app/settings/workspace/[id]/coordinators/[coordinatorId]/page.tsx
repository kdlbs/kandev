"use client";

import { CoordinatorEditorPage } from "@/components/coordinators/coordinator-editor-page";

type Props = {
  workspaceId: string;
  coordinatorId: string;
};

export default function CoordinatorEditorRoutePage({ workspaceId, coordinatorId }: Props) {
  return <CoordinatorEditorPage workspaceId={workspaceId} coordinatorId={coordinatorId} />;
}
