import type { Editor } from "@tiptap/core";
import type { ContextFile } from "@/lib/state/context-files-store";
import type { TaskMentionData } from "@/hooks/use-inline-mention";

export function getEditorMentions(editor: Editor | null): ContextFile[] {
  if (!editor) return [];
  const mentions: ContextFile[] = [];
  editor.state.doc.descendants((node) => {
    if (node.type.name !== "contextMention") return;
    const { kind, path, label } = node.attrs;
    if (kind === "file" || kind === "prompt" || kind === "plan") {
      mentions.push({ path, name: label, pinned: false });
    }
  });
  return mentions;
}

export function getEditorTaskMentions(editor: Editor | null): TaskMentionData[] {
  if (!editor) return [];
  const seen = new Set<string>();
  const mentions: TaskMentionData[] = [];
  editor.state.doc.descendants((node) => {
    if (node.type.name !== "contextMention") return;
    const { kind, label, taskId, workflowId, workflowStepId, taskState } = node.attrs;
    if (kind !== "task" || !taskId || !workflowId || !workflowStepId || seen.has(taskId)) return;
    seen.add(taskId);
    mentions.push({
      taskId,
      title: label ?? taskId,
      workflowId,
      workflowStepId,
      state: taskState ?? null,
    });
  });
  return mentions;
}
