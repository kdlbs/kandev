import type { PluginComposerSlotProps, PluginRegistry } from "../src/index";

type PluginComponent<Props> = (props: Props) => null;

const TaskCreateActions: PluginComponent<{ slotProps?: PluginComposerSlotProps }> = ({
  slotProps,
}) => {
  slotProps?.registerTaskCreatedHandler?.((task) => {
    const taskId: string = task.id;
    const workspaceId: string = task.workspace_id;
    void taskId;
    void workspaceId;
  });
  return null;
};

declare const registry: PluginRegistry;
registry.registerComponent("task-create-input-actions", TaskCreateActions);
