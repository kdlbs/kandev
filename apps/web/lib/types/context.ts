import type { DiffComment, PlanComment, PRFeedbackComment } from "@/lib/state/slices/comments";
import type { ImageAttachment } from "@/components/task/chat/image-attachment-preview";

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
  attachment: ImageAttachment;
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
  | PRFeedbackContextItem;

export type ContextItemKind = ContextItem["kind"];
