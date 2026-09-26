import { vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ConversationForkFormContext } from "./conversation-fork-types";

// i18n-exempt: test label for a mocked plugin composer callback.
export const PLUGIN_COMPOSER_LABEL = "Plugin composer action";
export const DESCRIPTION_INPUT_TEST_ID = "task-description-input";
// i18n-exempt: submitted value used only to assert fork prompt preservation.
export const TYPED_HANDOFF_PROMPT = "typed handoff prompt";

export const FORK_CONTEXT: ConversationForkFormContext = {
  snapshot: {
    descriptor: {
      id: "fork-1",
      source_task_id: "task-source",
      source_session_id: "session-source",
      source_message_id: "message-source",
      source_revision: 1,
      compiler_version: "v1",
      content_hash: "hash-1",
      message_count: 1,
      text_bytes: 20,
      omissions: {},
      attachments: [],
      estimate: { estimated_tokens: 5, method: "o200k", attachments_unmeasured: false },
      created_at: "2026-09-23T10:00:00Z",
      expires_at: "2026-09-24T10:00:00Z",
      state: "draft",
    },
    // i18n-exempt: snapshot content fixture rendered only in component tests.
    content: { content: "Frozen source history", content_hash: "hash-1", compiler_version: "v1" },
  },
  snapshotError: null,
  source: {
    taskId: "task-source",
    sessionId: "session-source",
    // i18n-exempt: source task title fixture rendered only in component tests.
    title: "Source task",
    revision: 1,
    cutoffMessageId: "message-source",
    cutoffTurnComplete: true,
    boundaries: [],
    attachments: [],
  },
  selection: { includeToolEvidence: false, attachmentIds: [] },
  creationRequestId: "create-1",
  attachmentsLoading: false,
  onPreview: vi.fn(),
  onRemove: vi.fn(),
  onApplySelection: vi.fn().mockResolvedValue(true),
  onRangeStartChange: vi.fn(),
  onModelChange: vi.fn(),
  onConsumed: vi.fn(),
};

export const BASE_PROFILE: AgentProfileOption = {
  id: "profile-1",
  label: "Profile 1",
  agent_name: "agent-1",
  agent_id: "agent-id-1",
  cli_passthrough: false,
  enabled: true,
};
