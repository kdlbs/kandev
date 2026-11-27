/** Collapsing a question leaves its next relay refresh available. */
export function shouldEngageQuestion(event: { target: EventTarget; key?: string }): boolean {
  if (event.key === "Escape") return false;
  const target = event.target as HTMLElement;
  return !target.closest?.('[data-testid="clarification-collapse-toggle"]');
}
