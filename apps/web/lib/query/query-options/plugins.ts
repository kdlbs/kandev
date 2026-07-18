import { queryOptions } from "@tanstack/react-query";
import { getPluginConfig, listPlugins } from "@/lib/api/domains/plugins-api";
import { qk } from "../keys";
import { withSignal } from "./utils";

export function pluginsQueryOptions() {
  return queryOptions({
    queryKey: qk.plugins.all(),
    queryFn: ({ signal }) => listPlugins(withSignal(signal)),
  });
}

export function pluginConfigQueryOptions(pluginId: string) {
  return queryOptions({
    queryKey: qk.plugins.config(pluginId),
    queryFn: ({ signal }) => getPluginConfig(pluginId, withSignal(signal)),
    enabled: Boolean(pluginId),
  });
}
