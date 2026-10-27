import { MobileFab } from "@/components/kanban/mobile-fab";
import { MobileTasksCreateDialog, type MobileTaskStep } from "./mobile-tasks-create-dialog";

export function MobileTasksActions({
  workspaceId,
  workflowId,
  steps,
  open,
  showFab,
  onOpenChange,
  onCreated,
}: {
  workspaceId: string;
  workflowId: string | null;
  steps: MobileTaskStep[];
  open: boolean;
  showFab: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => Promise<void>;
}) {
  return (
    <>
      <div className={showFab ? undefined : "hidden"} aria-hidden={!showFab}>
        <MobileFab onClick={() => onOpenChange(true)} />
      </div>
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
