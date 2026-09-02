export type KubernetesTestRequest = {
  config: Record<string, string>;
  profile_config?: Record<string, string>;
};

export type KubernetesTestStep = {
  key: string;
  success: boolean;
  duration_ms: number;
  detail: string;
  error?: string;
};

export type KubernetesWarning = {
  path: string;
  message: string;
};

export type KubernetesTestResult = {
  success: boolean;
  server_version?: string;
  namespace?: string;
  steps: KubernetesTestStep[];
  warnings: KubernetesWarning[];
  error?: string;
};

export type KubernetesSession = {
  session_id: string;
  task_id: string;
  pod_name?: string;
  pod_phase?: string;
  container_state?: string;
  restarts: number;
  workspace_kind?: string;
  created_at?: string;
  failure_reason?: string;
};

export type KubernetesSessionImpact = {
  active_session_count: number;
};
