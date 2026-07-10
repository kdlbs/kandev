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
