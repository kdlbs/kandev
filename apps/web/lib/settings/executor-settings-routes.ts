type ExecutorRouteIdentity = {
  id: string;
  type?: string | null;
};

export function kubernetesExecutorSettingsPath(executorId: string): string {
  return `/settings/executors/k8s/${encodeURIComponent(executorId)}`;
}

export function executorConnectionSettingsPath(executor: ExecutorRouteIdentity): string {
  return executor.type === "k8s"
    ? kubernetesExecutorSettingsPath(executor.id)
    : `/settings/executor/${encodeURIComponent(executor.id)}`;
}

export function executorProfileSettingsPath(
  executor: ExecutorRouteIdentity,
  profileId: string,
): string {
  return executor.type === "k8s"
    ? `/settings/executors/${encodeURIComponent(profileId)}`
    : `${executorConnectionSettingsPath(executor)}/profile/${encodeURIComponent(profileId)}`;
}
