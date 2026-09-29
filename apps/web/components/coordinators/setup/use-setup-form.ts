"use client";

import { useState } from "react";
import { CODE_KEYS } from "../sections/control-error";
import {
  isStepValid,
  stepErrors,
  type SetupErrors,
  type SetupServerError,
  type SetupState,
  type SetupStepId,
} from "@/lib/coordinators/setup";

const NOT_ACCEPTED_KEY = "coordinator:setupErrorNotAccepted";

type ServerIssue = { step: SetupStepId; field: string; key: string };

function overlaps(field: string, path: string): boolean {
  return field === path || field.startsWith(path) || path.startsWith(field);
}

/**
 * The setup values, the fields the manager has edited or left, and the
 * server's last refusal. A client error shows only for a touched field; a
 * server refusal keeps its step invalid until a field it names is edited.
 */
export function useSetupForm(initial: SetupState) {
  const [state, setState] = useState(initial);
  const [touched, setTouched] = useState<ReadonlySet<string>>(new Set());
  const [issue, setIssue] = useState<ServerIssue | null>(null);

  const edit = (patch: Partial<SetupState>, paths: readonly string[]) => {
    setState((prev) => ({ ...prev, ...patch }));
    setTouched((prev) => new Set([...prev, ...paths]));
    setIssue((prev) =>
      prev && (prev.field === "" || paths.some((p) => overlaps(prev.field, p))) ? null : prev,
    );
  };

  const leave = (path: string) => setTouched((prev) => new Set([...prev, path]));

  const reject = (error: SetupServerError) => {
    if (!error.step) return;
    const key = (error.code && CODE_KEYS[error.code]) || NOT_ACCEPTED_KEY;
    setIssue({ step: error.step, field: error.field ?? "", key });
  };

  /** Message keys by field path for one step; "" is a step-level line. */
  const messages = (step: SetupStepId): SetupErrors => {
    const out: SetupErrors = {};
    for (const [path, key] of Object.entries(stepErrors(step, state))) {
      if (touched.has(path)) out[path] = key;
    }
    if (issue?.step === step) out[issue.field] = issue.key;
    return out;
  };

  const valid = (step: SetupStepId) => isStepValid(step, state) && issue?.step !== step;

  return { state, setState, edit, leave, reject, messages, valid };
}
