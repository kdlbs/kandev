import type { DiffComment, PlanComment } from '@/lib/state/slices/comments';
import type { ImageAttachment } from '@/components/task/chat/image-attachment-preview';

type ContextItemBase = {
  id: string;
  label: string;
  pinned?: boolean;
  onRemove?: () => void;
  onUnpin?: () => void;
};

export type PlanContextItem = ContextItemBase & {
  kind: 'plan';
  taskId?: string;
  onOpen: () => void;
};

export type FileContextItem = ContextItemBase & {
  kind: 'file';
  path: string;
  onOpen: (path: string) => void;
};

export type PromptContextItem = ContextItemBase & {
  kind: 'prompt';
  promptContent?: string;
  onClick: () => void;
};

export type CommentContextItem = ContextItemBase & {
  kind: 'comment';
  filePath: string;
  comments: DiffComment[];
  onRemoveComment: (id: string) => void;
  onOpen?: () => void;
};

export type PlanCommentContextItem = ContextItemBase & {
  kind: 'plan-comment';
  comments: PlanComment[];
  onOpen: () => void;
};

export type ImageContextItem = ContextItemBase & {
  kind: 'image';
  attachment: ImageAttachment;
};

export type ContextItem =
  | PlanContextItem
  | FileContextItem
  | PromptContextItem
  | CommentContextItem
  | PlanCommentContextItem
  | ImageContextItem;

export type ContextItemKind = ContextItem['kind'];
