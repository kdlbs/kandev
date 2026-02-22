import { GitHubSettings } from "@/components/github/github-settings";
import { StateProvider } from "@/components/state-provider";
import { fetchGitHubStatus, listReviewWatches } from "@/lib/api/domains/github-api";

export default async function WorkspaceGitHubPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  let initialState = {};

  try {
    const [status, watches] = await Promise.all([
      fetchGitHubStatus({ cache: "no-store" }).catch(() => null),
      listReviewWatches(id, { cache: "no-store" }).catch(() => ({ watches: [] as never[] })),
    ]);

    initialState = {
      githubStatus: {
        status: status ?? null,
        loaded: true,
        loading: false,
      },
      reviewWatches: {
        items: watches.watches ?? [],
        loaded: true,
        loading: false,
      },
    };
  } catch {
    initialState = {};
  }

  return (
    <StateProvider initialState={initialState}>
      <GitHubSettings workspaceId={id} />
    </StateProvider>
  );
}
