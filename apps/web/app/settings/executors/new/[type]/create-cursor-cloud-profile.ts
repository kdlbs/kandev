import {
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
} from "@/lib/api/domains/settings-api";
import type { Executor } from "@/lib/types/http";

export async function createCursorCloudProfile(
  payload: Parameters<typeof createExecutorProfile>[1],
): Promise<Executor> {
  const executor = await createExecutor({ name: payload.name, type: "cursor_cloud" });
  try {
    const profile = await createExecutorProfile(executor.id, payload);
    return {
      ...executor,
      type: "cursor_cloud",
      status: "active",
      is_system: false,
      profiles: [profile],
      created_at: profile.created_at,
      updated_at: profile.updated_at,
    };
  } catch (error) {
    // The hub lists profiles, so an executor without one cannot be retried there.
    await deleteExecutor(executor.id).catch(() => undefined);
    throw error;
  }
}
