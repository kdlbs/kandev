"use client";

import { CoordinatorAddPage } from "@/components/coordinators/coordinator-add-page";
import { CoordinatorSetup } from "@/components/coordinators/setup/coordinator-setup";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { hasScope, SCOPE } from "@/lib/types/team-access";

type Props = {
  workspaceId: string;
};

export default function NewCoordinatorPage({ workspaceId }: Props) {
  const phase2 = useFeature("coordinatorPhase2");
  const scopes = useAppStore(
    (state) => state.workspaces.items.find((w) => w.id === workspaceId)?.scopes,
  );
  if (phase2 && hasScope(scopes, SCOPE.workspaceManage)) {
    return <CoordinatorSetup workspaceId={workspaceId} />;
  }
  return <CoordinatorAddPage workspaceId={workspaceId} />;
}
