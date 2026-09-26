import { useCallback, useRef, useState } from "react";
import type { Dispatch, MutableRefObject, SetStateAction } from "react";
import { ApiError } from "@/lib/api/client";
import {
  createConversationForkDraft,
  discardConversationForkDraft,
  estimateConversationForkDraft,
  getConversationForkContent,
  getConversationForkDraft,
  listConversationForkCandidates,
} from "@/lib/api/domains/conversation-fork-api";
import type {
  ConversationForkAttachment,
  ConversationForkCandidates,
  ConversationForkContent,
  ConversationForkDescriptor,
  ConversationForkEstimate,
} from "@/lib/api/domains/conversation-fork-api";
import { listTaskSessionMessages, listSessionTurns } from "@/lib/api/domains/session-api";
import type { Message, Turn } from "@/lib/types/http";
import { generateUUID } from "@/lib/uuid";

const FORK_SOURCE_ROW_LIMIT = 10_000;
const FORK_SOURCE_PAGE_SIZE = 100;

export type ConversationForkBoundary = {
  message: Message;
  finalized: boolean;
};

export type ConversationForkSource = {
  taskId: string;
  sessionId: string;
  title: string;
  revision: number;
  cutoffMessageId: string;
  cutoffTurnComplete: boolean;
  boundaries: ConversationForkBoundary[];
  attachments: ConversationForkAttachment[];
};

export type ConversationForkSnapshot = {
  descriptor: ConversationForkDescriptor;
  content: ConversationForkContent;
};

export type ConversationForkSelection = {
  startMessageId?: string;
  includeToolEvidence: boolean;
  attachmentIds: string[];
  modelId?: string;
};

export type ConversationForkError = {
  code: string;
  expired: boolean;
};

type ConversationForkHookState = {
  source: ConversationForkSource | null;
  sourceLoading: boolean;
  attachmentsLoading: boolean;
  sourceError: ConversationForkError | null;
  snapshot: ConversationForkSnapshot | null;
  snapshotLoading: boolean;
  snapshotError: ConversationForkError | null;
  estimateLoading: boolean;
  estimateError: ConversationForkError | null;
};

type StateSetter = Dispatch<SetStateAction<ConversationForkHookState>>;
type Generation = MutableRefObject<number>;
type ForkActionContext = {
  setState: StateSetter;
  sourceGeneration: Generation;
  attachmentGeneration: Generation;
  snapshotGeneration: Generation;
  estimateGeneration: Generation;
  draftRequest: DraftRequestRef;
};
type DraftRequest = {
  key: string;
  requestId: string;
  descriptor?: ConversationForkDescriptor;
};
type DraftRequestRef = MutableRefObject<DraftRequest | null>;
type SnapshotDraftContext = {
  source: ConversationForkSource;
  selection: ConversationForkSelection;
  request: DraftRequest;
  draftRequest: DraftRequestRef;
  previousSnapshotId?: string;
  isCurrent: () => boolean;
};

const initialState: ConversationForkHookState = {
  source: null,
  sourceLoading: false,
  attachmentsLoading: false,
  sourceError: null,
  snapshot: null,
  snapshotLoading: false,
  snapshotError: null,
  estimateLoading: false,
  estimateError: null,
};

function forkError(error: unknown): ConversationForkError {
  if (error instanceof ApiError) {
    const body = error.body as { code?: unknown } | null;
    return {
      code: typeof body?.code === "string" ? body.code : `http_${error.status}`,
      expired: error.status === 410,
    };
  }
  return {
    code: error instanceof Error ? error.message : "conversation_fork_failed",
    expired: false,
  };
}

function isConversationMessage(message: Message): boolean {
  return (
    (message.author_type === "user" || message.author_type === "agent") &&
    (message.type === "message" || message.type === "content")
  );
}

function isFinalizedBoundary(message: Message, turnById: Map<string, Turn>): boolean {
  if (message.author_type === "user") return true;
  if (!message.turn_id) return false;
  return turnById.get(message.turn_id)?.completed_at != null;
}

async function readMessagesThroughCutoff(
  sessionId: string,
  cutoffMessageId: string,
): Promise<Message[]> {
  const messages: Message[] = [];
  let cursor: string | undefined;
  while (messages.length < FORK_SOURCE_ROW_LIMIT) {
    const page = await listTaskSessionMessages(sessionId, {
      limit: FORK_SOURCE_PAGE_SIZE,
      after: cursor,
      sort: "asc",
    });
    if (page.messages.length === 0) break;
    messages.push(...page.messages);
    const cutoffIndex = messages.findIndex((message) => message.id === cutoffMessageId);
    if (cutoffIndex >= 0) return messages.slice(0, cutoffIndex + 1);
    if (!page.has_more || !page.cursor || page.cursor === cursor) break;
    cursor = page.cursor;
  }
  throw new Error("conversation_fork_cutoff_unavailable");
}

async function readAllCandidateAttachments(
  sessionId: string,
  cutoffMessageId: string,
  firstPage: ConversationForkCandidates,
  startMessageId?: string,
): Promise<ConversationForkAttachment[]> {
  const attachments = [...firstPage.attachments];
  let page = firstPage;
  while (page.attachments_has_more && page.attachment_cursor) {
    if (attachments.length >= FORK_SOURCE_ROW_LIMIT) {
      throw new Error("conversation_fork_limit_exceeded");
    }
    page = await listConversationForkCandidates(sessionId, cutoffMessageId, {
      attachmentCursor: page.attachment_cursor,
      startMessageId,
    });
    attachments.push(...page.attachments);
  }
  return attachments;
}

async function readSource(
  sessionId: string,
  cutoffMessageId: string,
): Promise<ConversationForkSource> {
  const firstPage = await listConversationForkCandidates(sessionId, cutoffMessageId);
  const [messages, turns, attachments] = await Promise.all([
    readMessagesThroughCutoff(sessionId, cutoffMessageId),
    listSessionTurns(sessionId),
    readAllCandidateAttachments(sessionId, cutoffMessageId, firstPage),
  ]);
  const cutoffMessage = messages.find((message) => message.id === cutoffMessageId);
  if (!cutoffMessage) throw new Error("conversation_fork_cutoff_unavailable");
  const turnById = new Map(turns.turns.map((turn) => [turn.id, turn]));
  const boundaries = messages.filter(isConversationMessage).map((message) => ({
    message,
    finalized:
      message.id === cutoffMessageId
        ? firstPage.cutoff_turn_complete
        : isFinalizedBoundary(message, turnById),
  }));
  return {
    taskId: firstPage.task_id,
    sessionId: firstPage.session_id,
    title: firstPage.task_title,
    revision: firstPage.revision,
    cutoffMessageId,
    cutoffTurnComplete: firstPage.cutoff_turn_complete,
    boundaries,
    attachments,
  };
}

function selectionKey(
  source: ConversationForkSource,
  selection: ConversationForkSelection,
): string {
  return JSON.stringify({
    sessionId: source.sessionId,
    cutoffMessageId: source.cutoffMessageId,
    startMessageId: selection.startMessageId ?? "",
    includeToolEvidence: selection.includeToolEvidence,
    attachmentIds: [...selection.attachmentIds].sort(),
  });
}

async function readSnapshot(forkId: string): Promise<ConversationForkSnapshot> {
  const [descriptor, content] = await Promise.all([
    getConversationForkDraft(forkId),
    getConversationForkContent(forkId),
  ]);
  if (descriptor.content_hash !== content.content_hash) {
    throw new Error("conversation_fork_content_mismatch");
  }
  return { descriptor, content };
}

function withEstimate(
  snapshot: ConversationForkSnapshot,
  estimate: ConversationForkEstimate,
): ConversationForkSnapshot {
  return {
    ...snapshot,
    descriptor: { ...snapshot.descriptor, estimate },
  };
}

function useConversationForkSourceActions(
  source: ConversationForkSource | null,
  context: ForkActionContext,
) {
  const {
    setState,
    sourceGeneration,
    attachmentGeneration,
    snapshotGeneration,
    estimateGeneration,
    draftRequest,
  } = context;
  const loadSource = useCallback(
    async (sessionId: string, cutoffMessageId: string) => {
      const generation = ++sourceGeneration.current;
      attachmentGeneration.current += 1;
      snapshotGeneration.current += 1;
      estimateGeneration.current += 1;
      draftRequest.current = null;
      setState({ ...initialState, sourceLoading: true });
      try {
        const nextSource = await readSource(sessionId, cutoffMessageId);
        if (generation === sourceGeneration.current) {
          setState({ ...initialState, source: nextSource, sourceLoading: false });
        }
      } catch (error) {
        if (generation === sourceGeneration.current) {
          setState({ ...initialState, sourceLoading: false, sourceError: forkError(error) });
        }
      }
    },
    [
      attachmentGeneration,
      draftRequest,
      estimateGeneration,
      setState,
      snapshotGeneration,
      sourceGeneration,
    ],
  );

  const loadAttachments = useCallback(
    async (startMessageId?: string) => {
      if (!source) return;
      const generation = ++attachmentGeneration.current;
      setState((current) => ({ ...current, attachmentsLoading: true, sourceError: null }));
      try {
        const firstPage = await listConversationForkCandidates(
          source.sessionId,
          source.cutoffMessageId,
          { startMessageId },
        );
        const attachments = await readAllCandidateAttachments(
          source.sessionId,
          source.cutoffMessageId,
          firstPage,
          startMessageId,
        );
        if (generation === attachmentGeneration.current) {
          setState((current) => ({
            ...current,
            source:
              current.source?.sessionId === source.sessionId
                ? { ...current.source, attachments }
                : current.source,
            attachmentsLoading: false,
          }));
        }
      } catch (error) {
        if (generation === attachmentGeneration.current) {
          setState((current) => ({
            ...current,
            attachmentsLoading: false,
            sourceError: forkError(error),
          }));
        }
      }
    },
    [attachmentGeneration, setState, source],
  );

  return { loadSource, loadAttachments };
}

function eligibleSnapshotSource(
  source: ConversationForkSource | null,
): ConversationForkSource | null {
  const cutoff = source?.boundaries.find(
    (boundary) => boundary.message.id === source.cutoffMessageId,
  )?.message;
  return source && cutoff && (cutoff.author_type === "user" || source.cutoffTurnComplete)
    ? source
    : null;
}

function getDraftRequest(
  source: ConversationForkSource,
  selection: ConversationForkSelection,
  forceNew: boolean,
  draftRequest: DraftRequestRef,
): DraftRequest {
  const key = selectionKey(source, selection);
  if (forceNew || draftRequest.current?.key !== key) {
    draftRequest.current = { key, requestId: generateUUID() };
  }
  return draftRequest.current!;
}

function discardStaleDraft(
  descriptor: ConversationForkDescriptor | undefined,
  request: DraftRequest,
  draftRequest: DraftRequestRef,
  previousSnapshotId?: string,
) {
  if (descriptor && request !== draftRequest.current && descriptor.id !== previousSnapshotId) {
    void discardConversationForkDraft(descriptor.id).catch(() => undefined);
  }
}

async function createSnapshotDraft(
  context: SnapshotDraftContext,
): Promise<ConversationForkSnapshot | null> {
  const { source, selection, request, draftRequest, previousSnapshotId, isCurrent } = context;
  let descriptor = request.descriptor;
  try {
    descriptor ??= await createConversationForkDraft(source.sessionId, {
      cutoff_message_id: source.cutoffMessageId,
      start_message_id: selection.startMessageId,
      draft_request_id: request.requestId,
      include_tool_evidence: selection.includeToolEvidence,
      attachment_ids: selection.attachmentIds,
    });
    request.descriptor = descriptor;
    if (!isCurrent()) {
      discardStaleDraft(descriptor, request, draftRequest, previousSnapshotId);
      return null;
    }
    const content = await getConversationForkContent(descriptor.id);
    if (descriptor.content_hash !== content.content_hash) {
      throw new Error("conversation_fork_content_mismatch");
    }
    if (!isCurrent()) {
      discardStaleDraft(descriptor, request, draftRequest, previousSnapshotId);
      return null;
    }
    return { descriptor, content };
  } catch (error) {
    if (!isCurrent()) discardStaleDraft(descriptor, request, draftRequest, previousSnapshotId);
    throw error;
  }
}

function useConversationForkSnapshotCreation(
  source: ConversationForkSource | null,
  currentSnapshot: ConversationForkSnapshot | null,
  context: ForkActionContext,
) {
  const { setState, snapshotGeneration, estimateGeneration, draftRequest } = context;
  const createSnapshot = useCallback(
    async (selection: ConversationForkSelection, options?: { forceNew?: boolean }) => {
      const eligibleSource = eligibleSnapshotSource(source);
      if (!eligibleSource) return null;
      const previousSnapshotId = currentSnapshot?.descriptor.id;
      const request = getDraftRequest(
        eligibleSource,
        selection,
        Boolean(options?.forceNew),
        draftRequest,
      );
      const generation = ++snapshotGeneration.current;
      estimateGeneration.current += 1;
      setState((current) => ({
        ...current,
        snapshotLoading: true,
        snapshotError: null,
        estimateError: null,
      }));
      try {
        const snapshot = await createSnapshotDraft({
          source: eligibleSource,
          selection,
          request,
          draftRequest,
          previousSnapshotId,
          isCurrent: () => generation === snapshotGeneration.current,
        });
        if (!snapshot) return null;
        if (generation === snapshotGeneration.current) {
          setState((current) => ({
            ...current,
            snapshot,
            snapshotLoading: false,
            snapshotError: null,
          }));
          if (previousSnapshotId && previousSnapshotId !== snapshot.descriptor.id) {
            void discardConversationForkDraft(previousSnapshotId).catch(() => undefined);
          }
        }
        return generation === snapshotGeneration.current ? snapshot : null;
      } catch (error) {
        if (generation === snapshotGeneration.current) {
          setState((current) => ({
            ...current,
            snapshotLoading: false,
            snapshotError: forkError(error),
          }));
        }
        return null;
      }
    },
    [currentSnapshot, draftRequest, estimateGeneration, setState, snapshotGeneration, source],
  );

  return createSnapshot;
}

function useConversationForkSnapshotActions(
  currentSnapshot: ConversationForkSnapshot | null,
  context: ForkActionContext,
) {
  const { setState, snapshotGeneration, estimateGeneration, draftRequest } = context;
  const loadSnapshot = useCallback(
    async (forkId: string) => {
      const generation = ++snapshotGeneration.current;
      estimateGeneration.current += 1;
      setState((current) => ({ ...current, snapshotLoading: true, snapshotError: null }));
      try {
        const snapshot = await readSnapshot(forkId);
        if (generation === snapshotGeneration.current) {
          setState((current) => ({ ...current, snapshot, snapshotLoading: false }));
        }
        return generation === snapshotGeneration.current ? snapshot : null;
      } catch (error) {
        if (generation === snapshotGeneration.current) {
          setState((current) => ({
            ...current,
            snapshotLoading: false,
            snapshotError: forkError(error),
          }));
        }
        return null;
      }
    },
    [estimateGeneration, setState, snapshotGeneration],
  );

  const refreshEstimate = useCallback(
    async (modelId: string) => {
      if (!currentSnapshot || !modelId) return;
      const generation = ++estimateGeneration.current;
      setState((current) => ({ ...current, estimateLoading: true, estimateError: null }));
      try {
        const estimate = await estimateConversationForkDraft(
          currentSnapshot.descriptor.id,
          modelId,
        );
        if (generation === estimateGeneration.current) {
          setState((current) => ({
            ...current,
            snapshot: current.snapshot ? withEstimate(current.snapshot, estimate) : null,
            estimateLoading: false,
          }));
        }
      } catch (error) {
        if (generation === estimateGeneration.current) {
          setState((current) => ({
            ...current,
            estimateLoading: false,
            estimateError: forkError(error),
          }));
        }
      }
    },
    [currentSnapshot, estimateGeneration, setState],
  );

  const discardSnapshot = useCallback(async () => {
    const forkIds = new Set(
      [currentSnapshot?.descriptor.id, draftRequest.current?.descriptor?.id].filter(
        (forkId): forkId is string => Boolean(forkId),
      ),
    );
    snapshotGeneration.current += 1;
    estimateGeneration.current += 1;
    for (const forkId of forkIds) {
      try {
        await discardConversationForkDraft(forkId);
      } catch {
        // Draft expiry and an already attached destination both make discard a no-op.
      }
    }
    draftRequest.current = null;
    setState((current) => ({ ...current, snapshot: null, snapshotLoading: false }));
  }, [currentSnapshot, draftRequest, estimateGeneration, setState, snapshotGeneration]);

  return { loadSnapshot, refreshEstimate, discardSnapshot };
}

export function useConversationFork() {
  const [state, setState] = useState(initialState);
  const sourceGeneration = useRef(0);
  const attachmentGeneration = useRef(0);
  const snapshotGeneration = useRef(0);
  const estimateGeneration = useRef(0);
  const draftRequest = useRef<DraftRequest | null>(null);
  const context = {
    setState,
    sourceGeneration,
    attachmentGeneration,
    snapshotGeneration,
    estimateGeneration,
    draftRequest,
  };
  const sourceActions = useConversationForkSourceActions(state.source, context);
  const createSnapshot = useConversationForkSnapshotCreation(state.source, state.snapshot, context);
  const snapshotActions = useConversationForkSnapshotActions(state.snapshot, context);
  const reset = useCallback(() => {
    sourceGeneration.current += 1;
    attachmentGeneration.current += 1;
    snapshotGeneration.current += 1;
    estimateGeneration.current += 1;
    draftRequest.current = null;
    setState(initialState);
  }, []);

  return {
    ...state,
    ...sourceActions,
    createSnapshot,
    ...snapshotActions,
    reset,
  };
}
