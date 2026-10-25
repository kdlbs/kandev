import { MobileFab } from "@/components/kanban/mobile-fab";
import { MobileTasksCreateDialog, type MobileTaskStep } from "./mobile-tasks-create-dialog";

export function MobileTasksActions({
  workspaceId,
  workflowId,
  steps,
  open,
  onOpenChange,
  onCreated,
}: {
  workspaceId: string;
  workflowId: string | null;
  steps: MobileTaskStep[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => Promise<void>;
}) {
  return (
    <>
      <MobileFab onClick={() => onOpenChange(true)} />
      <MobileTasksCreateDialog
        open={open}
        onOpenChange={onOpenChange}
        workspaceId={workspaceId}
        workflowId={workflowId}
        steps={steps}
        onCreated={onCreated}
      />
    </>
  );
}
