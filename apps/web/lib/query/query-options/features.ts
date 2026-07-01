import { queryOptions } from "@tanstack/react-query";
import { fetchFeatureFlags } from "@/lib/api/domains/features-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function featureFlagsQueryOptions() {
  return queryOptions({
    queryKey: qk.features(),
    queryFn: ({ signal }) => fetchFeatureFlags(withSignal(signal)),
  });
}
