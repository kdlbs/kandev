export type RepositoryCheckoutOptions = {
  version: 1;
  download_mode: "standard" | "on_demand";
  sparse_directories: string[];
};
export type RepositoryCheckoutCapabilities = {
  on_demand: boolean;
  sparse: boolean;
  reason?:
    | "provider_unsupported"
    | "executor_required"
    | "preparation_unsupported"
    | "credentials_unsupported";
};
