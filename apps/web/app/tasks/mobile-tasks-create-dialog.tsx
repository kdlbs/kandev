import { TaskCreateDialog } from "@/components/task-create-dialog";

export type MobileTaskStep = {
  id: string;
  title: string;
  events?: {
    on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
    on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
  };
};

export function MobileTasksCreateDialog({
  open,
  onOpenChange,
  workspaceId,
  workflowId,
  steps,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  workflowId: string | null;
  steps: MobileTaskStep[];
  onCreated: () => Promise<void>;
}) {
  return (
    <TaskCreateDialog
      open={open}
      onOpenChange={onOpenChange}
      mode="create"
      workspaceId={workspaceId}
      workflowId={workflowId}
      defaultStepId={steps[0]?.id ?? null}
      steps={steps}
      onSuccess={() => {
        onOpenChange(false);
        void onCreated();
      }}
    />
  );
}
