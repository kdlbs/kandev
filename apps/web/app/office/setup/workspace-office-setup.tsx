import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import { useKanbanOnboardingComplete } from "@/hooks/use-kanban-onboarding-complete";
import { useRouter } from "@/lib/routing/client-router";
import { officeSetupHref } from "@/lib/settings/office-setup";
export function WorkspaceOfficeSetup() {
  const completed = useKanbanOnboardingComplete();
  const router = useRouter();
  const workspaceId = useAppStore(
    (s) => s.workspaces.activeId ?? s.workspaces.items[0]?.id ?? null,
  );
  useEffect(() => {
    router.replace(officeSetupHref(completed, workspaceId));
  }, [completed, workspaceId, router]);
  return null;
}
