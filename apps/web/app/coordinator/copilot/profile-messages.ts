export type Translate = (key: string) => string;

export type ProfileMessages = {
  agentMessage: string | null;
  executorMessage: string | null;
};

/**
 * Maps a coordinator GET or a route 409's two profile statuses to the copy
 * of docs/specs/coordinator/system-design/coordinators.md#validation: one
 * message per field that is not `ok`, both shown together when neither is.
 * A status value the client does not know (a future backend value, or a
 * malformed body) is shown as that field's `missing` message.
 */
export function profileStatusMessages(
  agentStatus: string,
  executorStatus: string,
  t: Translate,
): ProfileMessages {
  return {
    agentMessage: agentStatus === "ok" ? null : agentMessageFor(agentStatus, t),
    executorMessage:
      executorStatus === "ok" ? null : t("coordinator:copilotExecutorMissingMessage"),
  };
}

function agentMessageFor(status: string, t: Translate): string {
  return status === "passthrough"
    ? t("coordinator:copilotAgentPassthroughMessage")
    : t("coordinator:copilotAgentMissingMessage");
}
