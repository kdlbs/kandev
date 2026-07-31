/** The WS gateway forwards run events verbatim: id at top level, row under `event`. */
export type RunEventAppendedPayload = {
  run_id: string;
  event: {
    seq: number;
    event_type: string;
    level: string;
    payload: string;
    created_at: string;
  };
};
