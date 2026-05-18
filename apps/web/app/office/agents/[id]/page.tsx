import { redirect } from "next/navigation";

type AgentDetailPageProps = {
  params: Promise<{ id: string }>;
};

/**
 * The bare agent URL redirects to the dashboard sub-route. Every
 * section under `/office/agents/[id]` is a real bookmarkable URL —
 * see `layout.tsx` for the tab strip and `<segment>/page.tsx` files
 * for each section's body.
 */
export default async function AgentDetailPage({ params }: AgentDetailPageProps) {
  const { id } = await params;
  redirect(`/office/agents/${id}/dashboard`);
}
