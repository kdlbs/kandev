export type AppConfig = {
  apiBaseUrl: string;
};

const DEFAULT_API_BASE_URL = 'http://localhost:8080';

export function getBackendConfig(): AppConfig {
  if (typeof window === 'undefined') {
    return {
      apiBaseUrl: process.env.KANDEV_API_BASE_URL ?? DEFAULT_API_BASE_URL,
    };
  }
  return {
    apiBaseUrl: process.env.NEXT_PUBLIC_KANDEV_API_BASE_URL ?? DEFAULT_API_BASE_URL,
  };
}
