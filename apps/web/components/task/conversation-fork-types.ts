import type {
  ConversationForkError,
  ConversationForkSelection,
  ConversationForkSnapshot,
  ConversationForkSource,
} from "@/hooks/domains/task/use-conversation-fork";

export type ConversationForkFormContext = {
  snapshot: ConversationForkSnapshot;
  snapshotError: ConversationForkError | null;
  source: ConversationForkSource;
  selection: ConversationForkSelection;
  creationRequestId: string;
  attachmentsLoading: boolean;
  onPreview: () => void;
  onRemove: () => void;
  onApplySelection: (selection: ConversationForkSelection, forceNew?: boolean) => Promise<boolean>;
  onRangeStartChange: (startMessageId?: string) => void;
  onModelChange: (modelId: string) => void;
  onConsumed: () => void;
};
