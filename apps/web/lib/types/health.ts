export type HealthSeverity = "warning" | "error" | "info";

export type HealthIssue = {
  id: string;
  category: string;
  title: string;
  message: string;
  severity: HealthSeverity;
  fix_url: string;
  fix_label: string;
};

export type SystemHealthResponse = {
  healthy: boolean;
  issues: HealthIssue[];
};
