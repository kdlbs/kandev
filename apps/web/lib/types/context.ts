import type { DiffComment, PlanComment, PRFeedbackComment } from "@/lib/state/slices/comments";
import type { FileAttachment } from "@/components/task/chat/file-attachment";

type ContextItemBase = {
  id: string;
  label: string;
  pinned?: boolean;
  onRemove?: () => void;
  onUnpin?: () => void;
};

export type PlanContextItem = ContextItemBase & {
  kind: "plan";
  taskId?: string;
  onOpen: () => void;
};

export type FileContextItem = ContextItemBase & {
  kind: "file";
  path: string;
  onOpen: (path: string) => void;
};

export type PromptContextItem = ContextItemBase & {
  kind: "prompt";
  promptContent?: string;
  onClick: () => void;
};

export type CommentContextItem = ContextItemBase & {
  kind: "comment";
  filePath: string;
  comments: DiffComment[];
  onRemoveComment: (id: string) => void;
  onOpen?: () => void;
};

export type PlanCommentContextItem = ContextItemBase & {
  kind: "plan-comment";
  comments: PlanComment[];
  onOpen: () => void;
};

export type ImageContextItem = ContextItemBase & {
  kind: "image";
  attachment: FileAttachment;
};

export type FileAttachmentContextItem = ContextItemBase & {
  kind: "file-attachment";
  attachment: FileAttachment;
};

export type PRFeedbackContextItem = ContextItemBase & {
  kind: "pr-feedback";
  comments: PRFeedbackComment[];
  onRemoveComment: (id: string) => void;
};

export type ContextItem =
  | PlanContextItem
  | FileContextItem
  | PromptContextItem
  | CommentContextItem
  | PlanCommentContextItem
  | ImageContextItem
  | FileAttachmentContextItem
  | PRFeedbackContextItem;

export type ContextItemKind = ContextItem["kind"];
