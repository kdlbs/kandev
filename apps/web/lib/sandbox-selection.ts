import type { SandboxRevision } from "./types/sandbox";

export const SANDBOX_INHERIT = "__inherit__";

export function sandboxSelectionChoices(
  eligible: SandboxRevision[],
  storedId: string,
  storedLabel: string,
  inheritLabel: string,
) {
  const choices: { value: string; label: string; disabled?: boolean }[] = [
    { value: SANDBOX_INHERIT, label: inheritLabel },
    ...eligible
      .filter((item) => !item.revoked)
      .map((item) => ({ value: item.id, label: `${item.policy.name} (${item.number})` })),
  ];
  if (storedId && !choices.some((item) => item.value === storedId))
    choices.push({ value: storedId, label: storedLabel, disabled: true });
  return choices;
}

export function canSaveSandboxSelection(
  revisionId: string,
  eligible: SandboxRevision[],
  ready: boolean,
) {
  return ready && (!revisionId || eligible.some((item) => item.id === revisionId && !item.revoked));
}
