import type { ContextFile } from "@/lib/state/context-files-store";
import { getFileName } from "@/lib/utils/file-path";
import type { ContextItem } from "@/lib/types/context";
import type { DiffComment } from "@/lib/diff/types";
import type { PlanComment } from "@/lib/state/slices/comments";

const PLAN_CONTEXT_PATH = "plan:context";

export type BuildContextItemsParams = {
  planContextEnabled: boolean;
  contextFiles: ContextFile[];
  resolvedSessionId: string | null;
  removeContextFile: (sid: string, path: string) => void;
  unpinFile: (sid: string, path: string) => void;
  addPlan: () => void;
  promptsMap: Map<string, { content: string }>;
  onOpenFile?: (path: string) => void;
  pendingCommentsByFile: Record<string, DiffComment[]>;
  handleRemoveCommentFile: (filePath: string) => void;
  handleRemoveComment: (commentId: string) => void;
  onOpenFileAtLine?: (filePath: string) => void;
  planComments: PlanComment[];
  handleClearPlanComments: () => void;
  taskId: string | null;
};

type FileItemHelpers = {
  sid: string | null;
  removeContextFile: (sid: string, path: string) => void;
  unpinFile: (sid: string, path: string) => void;
};

function makeRemoveHandler(
  sid: string | null,
  path: string,
  removeContextFile: (sid: string, path: string) => void,
) {
  return sid ? () => removeContextFile(sid, path) : undefined;
}

function makeUnpinHandler(
  pinned: boolean | undefined,
  sid: string | null,
  path: string,
  unpinFile: (sid: string, path: string) => void,
) {
  return pinned && sid ? () => unpinFile(sid, path) : undefined;
}

function buildPlanContextItem(params: BuildContextItemsParams): ContextItem | null {
  if (!params.planContextEnabled) return null;
  const { contextFiles, resolvedSessionId: sid, removeContextFile, unpinFile, addPlan, taskId } =
    params;
  const planFile = contextFiles.find((f) => f.path === PLAN_CONTEXT_PATH);
  return {
    kind: "plan",
    id: PLAN_CONTEXT_PATH,
    label: "Plan",
    taskId: taskId ?? undefined,
    pinned: planFile?.pinned,
    onRemove: sid ? () => removeContextFile(sid, PLAN_CONTEXT_PATH) : undefined,
    onUnpin: planFile?.pinned && sid ? () => unpinFile(sid, PLAN_CONTEXT_PATH) : undefined,
    onOpen: addPlan,
  };
}

function buildPromptContextItem(
  f: ContextFile,
  helpers: FileItemHelpers,
  promptsMap: Map<string, { content: string }>,
): ContextItem {
  const prompt = promptsMap.get(f.path.replace("prompt:", ""));
  return {
    kind: "prompt",
    id: f.path,
    label: f.name,
    pinned: f.pinned,
    onRemove: makeRemoveHandler(helpers.sid, f.path, helpers.removeContextFile),
    onUnpin: makeUnpinHandler(f.pinned, helpers.sid, f.path, helpers.unpinFile),
    promptContent: prompt?.content,
    onClick: () => {
      /* navigate to settings/prompts if desired */
    },
  };
}

function buildFileContextItem(
  f: ContextFile,
  helpers: FileItemHelpers,
  onOpenFile: ((path: string) => void) | undefined,
): ContextItem {
  return {
    kind: "file",
    id: f.path,
    label: f.name,
    path: f.path,
    pinned: f.pinned,
    onRemove: makeRemoveHandler(helpers.sid, f.path, helpers.removeContextFile),
    onUnpin: makeUnpinHandler(f.pinned, helpers.sid, f.path, helpers.unpinFile),
    onOpen: onOpenFile ?? (() => {}),
  };
}

function buildFileAndPromptItems(params: BuildContextItemsParams): ContextItem[] {
  const { contextFiles, resolvedSessionId: sid, removeContextFile, unpinFile, promptsMap, onOpenFile } =
    params;
  const helpers: FileItemHelpers = { sid, removeContextFile, unpinFile };
  const items: ContextItem[] = [];
  for (const f of contextFiles) {
    if (f.path === PLAN_CONTEXT_PATH) continue;
    items.push(
      f.path.startsWith("prompt:")
        ? buildPromptContextItem(f, helpers, promptsMap)
        : buildFileContextItem(f, helpers, onOpenFile),
    );
  }
  return items;
}

function buildCommentItems(params: BuildContextItemsParams): ContextItem[] {
  const { pendingCommentsByFile, handleRemoveCommentFile, handleRemoveComment, onOpenFileAtLine } =
    params;
  const items: ContextItem[] = [];
  if (!pendingCommentsByFile) return items;
  for (const [filePath, comments] of Object.entries(pendingCommentsByFile)) {
    if (comments.length === 0) continue;
    const fileName = getFileName(filePath);
    items.push({
      kind: "comment",
      id: `comment:${filePath}`,
      label: `${fileName} (${comments.length})`,
      filePath,
      comments,
      onRemove: () => handleRemoveCommentFile(filePath),
      onRemoveComment: (cid) => handleRemoveComment(cid),
      onOpen: onOpenFileAtLine ? () => onOpenFileAtLine(filePath) : undefined,
    });
  }
  return items;
}

/** Sort: pinned first, then by kind order, then by label */
const KIND_ORDER: Record<string, number> = {
  plan: 0,
  file: 1,
  prompt: 2,
  comment: 3,
  "plan-comment": 4,
  image: 5,
};

export function contextItemSortFn(a: ContextItem, b: ContextItem): number {
  const aPinned = a.pinned ? 0 : 1;
  const bPinned = b.pinned ? 0 : 1;
  if (aPinned !== bPinned) return aPinned - bPinned;
  const aKind = KIND_ORDER[a.kind] ?? 99;
  const bKind = KIND_ORDER[b.kind] ?? 99;
  if (aKind !== bKind) return aKind - bKind;
  return a.label.localeCompare(b.label);
}

export function buildContextItems(params: BuildContextItemsParams): ContextItem[] {
  const items: ContextItem[] = [];
  const planItem = buildPlanContextItem(params);
  if (planItem) items.push(planItem);
  items.push(...buildFileAndPromptItems(params));
  items.push(...buildCommentItems(params));

  if (params.planComments.length > 0) {
    items.push({
      kind: "plan-comment",
      id: "plan-comments",
      label: `${params.planComments.length} plan comment${params.planComments.length !== 1 ? "s" : ""}`,
      comments: params.planComments,
      onRemove: params.handleClearPlanComments,
      onOpen: params.addPlan,
    });
  }

  return items.sort(contextItemSortFn);
}
