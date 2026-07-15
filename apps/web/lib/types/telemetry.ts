export type TelemetryConsentStatus = "unasked" | "granted" | "denied";

export type TelemetryConsentState = {
  status: TelemetryConsentStatus;
  /** Anonymous install UUID; present only after consent is granted. */
  install_id?: string;
  /** True when DO_NOT_TRACK or e2e test mode hard-disables telemetry. */
  env_disabled: boolean;
};

export type TelemetryUIEventName = "ui_page_viewed" | "ui_action" | "feature_used";

export type TelemetryUIEvent = {
  name: TelemetryUIEventName;
  properties: Record<string, string>;
};

export type TelemetryEventsResponse = {
  accepted: number;
};
