export type EntityReferenceGroupStatus =
  | "ok"
  | "not_configured"
  | "unauthorized"
  | "rate_limited"
  | "timeout"
  | "upstream_error"
  | "unsupported_scope";

export type EntityReference = {
  version: number;
  ref: string;
  provider: string;
  kind: string;
  id: string;
  key?: string;
  title: string;
  url: string;
  scope: string;
};

export type EntityReferenceSearchGroup = {
  source: string;
  provider: string;
  kind: string;
  display_name: string;
  kind_label: string;
  status: EntityReferenceGroupStatus;
  results: EntityReference[];
};

export type EntityReferenceSearchResponse = {
  query: string;
  groups: EntityReferenceSearchGroup[];
};
