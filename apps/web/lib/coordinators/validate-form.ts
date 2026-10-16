// Mirrors internal/coordinator/validate.go's name bound (1 to 60 Unicode
// code points, trimmed) so the Add button's enabled state and the server's
// 400 never disagree.
export const COORDINATOR_NAME_MAX_LENGTH = 60;

export function isValidCoordinatorName(name: string): boolean {
  const trimmed = name.trim();
  if (trimmed.length === 0) return false;
  return Array.from(trimmed).length <= COORDINATOR_NAME_MAX_LENGTH;
}

export type CoordinatorFormFields = {
  name: string;
  agentProfileId: string;
  executorProfileId: string;
  taskAgentProfileId: string;
  taskExecutorProfileId: string;
};

export function canAddCoordinator(form: CoordinatorFormFields): boolean {
  return (
    isValidCoordinatorName(form.name) &&
    form.agentProfileId.length > 0 &&
    form.executorProfileId.length > 0 &&
    form.taskAgentProfileId.length > 0 &&
    form.taskExecutorProfileId.length > 0
  );
}
