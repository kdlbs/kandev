import type { DiffComment } from '@/lib/diff/types';
import type { DocumentComment } from '@/lib/state/slices/ui/types';
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
  comments: DocumentComment[];
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
