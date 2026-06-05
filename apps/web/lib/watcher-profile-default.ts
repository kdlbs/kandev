// Shared sentinel for the "(use step default)" option in the Agent/Executor
// Profile selects across the four watcher dialogs (Linear, Jira, GitHub issue,
// GitHub review). The form stores "" to mean "fall back to the workflow step's
// default profile", and the create/update payloads pass "" through. Radix
// disallows <SelectItem value="">, so the dropdown item carries this sentinel
// value and maps back to "" on change.
export const STEP_DEFAULT = "__step_default__";
export const STEP_DEFAULT_LABEL = "(use step default)";

// Map a select value back to the stored profile id, collapsing the sentinel to
// "" so the payload keeps signalling "use step default".
export function resolveProfileId(value: string): string {
  return value === STEP_DEFAULT ? "" : value;
}
