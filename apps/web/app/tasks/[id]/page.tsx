import { redirect } from "next/navigation";

/**
 * Compatibility alias for `/tasks/:id`.
 *
 * `/t/:taskId` remains the canonical kanban task-detail route; keep this
 * redirect so links produced during the task-model-unification work continue
 * to resolve.
 */
export default async function TasksDetailAliasPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const { id } = await params;
  const search = await searchParams;
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(search)) {
    if (value === undefined) continue;
    if (Array.isArray(value)) {
      for (const v of value) query.append(key, v);
    } else {
      query.set(key, value);
    }
  }
  const queryString = query.toString();
  redirect(queryString ? `/t/${id}?${queryString}` : `/t/${id}`);
}
