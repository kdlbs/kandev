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
  selection_id?: string;
  association_mode?: EntityReferenceAssociationMode;
  registration_status?: EntityReferenceRegistrationStatus;
  registration_error_code?: string;
};

export type EntityReferenceAssociationMode = "link_on_send" | "context_only";
export type EntityReferenceRegistrationStatus =
  | "pending"
  | "linked"
  | "failed"
  | "suppressed"
  | "uncertain"
  | "unlinked";

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
