import type { BootPayload } from "./boot-payload";

export type InitialPageProps = {
  initialTaskId?: string;
  initialSessionId?: string;
};

export function getInitialPageProps(payload: BootPayload): InitialPageProps {
  const route = payload.route;
  if (route?.route !== "taskDetail") return {};

  return {
    initialTaskId: route.params?.taskId,
  };
}

export function readTaskId(pathname: string): string | undefined {
  for (const prefix of ["/t/", "/tasks/"]) {
    if (!pathname.startsWith(prefix)) continue;
    const suffix = pathname.slice(prefix.length);
    if (!suffix || suffix.includes("/")) return undefined;
    return decodeURIComponent(suffix);
  }
  return undefined;
}
