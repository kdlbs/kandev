export type HandoffPreset = {
  sourceSessionId: string;
  targetProfileId: string;
};

export function buildHandoffInitialState(handoff: HandoffPreset): {
  selectedProfileId: string;
  contextValue: string;
} {
  return {
    selectedProfileId: handoff.targetProfileId,
    contextValue: summarizeContextValue(handoff.sourceSessionId),
  };
}

export function summarizeContextValue(sessionId: string): string {
  return `summarize:${sessionId}`;
}
