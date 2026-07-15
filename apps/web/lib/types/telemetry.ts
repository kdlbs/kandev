export type TelemetryConsentStatus = "unasked" | "granted" | "denied";

export type TelemetryConsentState = {
  status: TelemetryConsentStatus;
  /** Anonymous install UUID; present only after consent is granted. */
  install_id?: string;
  /** True when DO_NOT_TRACK / KANDEV_TELEMETRY=off hard-disable telemetry. */
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
