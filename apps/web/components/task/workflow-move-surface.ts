export const WORKFLOW_MOVE_OPTIONS_SELECTOR = '[data-testid="workflow-move-options"]';

export function isWorkflowMoveOptionsTarget(target: EventTarget | null): boolean {
  return target instanceof Element && Boolean(target.closest(WORKFLOW_MOVE_OPTIONS_SELECTOR));
}

export function keepWorkflowMoveSurfaceOpen(event: {
  target: EventTarget | null;
  preventDefault: () => void;
}): void {
  if (isWorkflowMoveOptionsTarget(event.target)) event.preventDefault();
}
