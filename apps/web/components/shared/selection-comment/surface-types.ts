import type { ReactNode } from "react";

export type SelectionPosition = {
  x: number;
  y: number;
};

export type CommentCallback = (
  comment: string,
  selectedText: string,
) => boolean | void | Promise<boolean | void>;

export type SelectionCommentSurfaceProps = {
  /** Plain-text evidence; quoted by default and used as the Drawer description. */
  selectedText: string;
  /** Desktop anchor in viewport pixels. */
  position: SelectionPosition;
  onAdd: CommentCallback;
  onAddAndRun?: CommentCallback;
  onClose: () => void;
  /** Opt-in Close control that leaves without saving; text is not required. */
  onCancel?: () => void;
  /** Keeps Add disabled, e.g. while the evidence is still being prepared. */
  addDisabled?: boolean;
  editingComment?: string;
  onDelete?: () => boolean | void | Promise<boolean | void>;
  /** Short tag such as a line range or page route. */
  contextLabel?: string;
  /** Replaces the quoted selectedText, e.g. a screenshot thumbnail. */
  evidence?: ReactNode;
  /** Controlled draft; both must be supplied to take effect. */
  draftComment?: string;
  onDraftChange?: (comment: string) => void;
  /** Accessible name for the comment field. */
  inputLabel?: string;
  testId?: string;
  inputTestId?: string;
  addButtonTestId?: string;
  runButtonTestId?: string;
  portalContainer?: HTMLElement | null;
  /** Fades and zooms in on open; false appears in place at once. Defaults to true. */
  animated?: boolean;
  errorMessage?: string | null;
  runDisabledReason?: string | null;
};
