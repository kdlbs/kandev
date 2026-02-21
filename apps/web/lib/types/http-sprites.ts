export interface SpritesInstance {
  name: string;
  health_status: "healthy" | "unhealthy" | "unknown";
  created_at: string;
  uptime_seconds: number;
}

export interface SpritesStatus {
  connected: boolean;
  token_configured: boolean;
  instance_count: number;
  error?: string;
}

export interface SpritesTestResult {
  success: boolean;
  steps: SpritesTestStep[];
  total_duration_ms: number;
  sprite_name: string;
  error?: string;
}

export interface SpritesTestStep {
  name: string;
  duration_ms: number;
  success: boolean;
  error?: string;
}
