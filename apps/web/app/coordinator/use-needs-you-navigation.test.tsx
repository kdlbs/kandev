import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import type {
  AttentionProposal,
  NeedsYouItem,
  NeedsYouProposalItem,
} from "@/lib/coordinator/attention";

let searchString = "";
const replace = vi.fn();

vi.mock("@/lib/routing/client-router", () => ({
  useSearchParams: () => new URLSearchParams(searchString),
  usePathname: () => "/workspaces/w-1/coordinator/c-1",
  useRouter: () => ({ replace }),
}));

import { useNeedsYouFormNavigation } from "./use-needs-you-navigation";

afterEach(() => {
  cleanup();
  replace.mockClear();
  searchString = "";
});

function proposalItem(id: string): NeedsYouProposalItem {
  return {
    kind: "proposal",
    id,
    referenceTimeMs: 0,
    ageMs: 0,
    proposal: { id } as AttentionProposal,
  };
}

function wrapper({ children }: { children: React.ReactNode }) {
  return <ToastProvider>{children}</ToastProvider>;
}

function renderNav(items: NeedsYouItem[], inputsLoaded: boolean) {
  return renderHook(() => useNeedsYouFormNavigation(items, inputsLoaded), { wrapper });
}

const PROPOSAL_EDIT_DEEP_LINK = "proposal=p-1&form=edit";

describe("useNeedsYouFormNavigation", () => {
  it("returns nothing to auto-open while inputs are still loading", () => {
    searchString = PROPOSAL_EDIT_DEEP_LINK;
    const { result } = renderNav([proposalItem("p-1")], false);
    expect(result.current.autoOpenProposalId).toBeNull();
    expect(result.current.autoOpenForm).toBeNull();
    expect(replace).not.toHaveBeenCalled();
  });

  it("hands back the matching item's form without clearing the query params yet", () => {
    searchString = PROPOSAL_EDIT_DEEP_LINK;
    const { result } = renderNav([proposalItem("p-1")], true);
    expect(result.current.autoOpenProposalId).toBe("p-1");
    expect(result.current.autoOpenForm).toBe("edit");
    // The matching item's own ProposalCard has not reported opening the
    // form yet (it may not even be mounted with a resolved row yet), so the
    // params must stay in place — clearing them here would hand a
    // still-mounting card an already-cleared autoOpenForm.
    expect(replace).not.toHaveBeenCalled();
  });

  it("clears the query params once the matching card reports onAutoFormOpened", () => {
    searchString = PROPOSAL_EDIT_DEEP_LINK;
    const { result } = renderNav([proposalItem("p-1")], true);
    result.current.onAutoFormOpened();
    expect(replace).toHaveBeenCalledExactlyOnceWith("/workspaces/w-1/coordinator/c-1");
  });

  it("only clears the query params once per proposal id, even if called again", () => {
    searchString = PROPOSAL_EDIT_DEEP_LINK;
    const { result } = renderNav([proposalItem("p-1")], true);
    result.current.onAutoFormOpened();
    result.current.onAutoFormOpened();
    expect(replace).toHaveBeenCalledOnce();
  });

  it("clears the query params immediately, without waiting, when the id is not a current item", () => {
    searchString = "proposal=missing&form=edit";
    const { result } = renderNav([proposalItem("p-1")], true);
    expect(result.current.autoOpenProposalId).toBe("missing");
    expect(replace).toHaveBeenCalledExactlyOnceWith("/workspaces/w-1/coordinator/c-1");
  });

  it("does nothing when there is no proposal/form deep link in the URL", () => {
    searchString = "";
    const { result } = renderNav([proposalItem("p-1")], true);
    expect(result.current.autoOpenProposalId).toBeNull();
    expect(result.current.autoOpenForm).toBeNull();
    expect(replace).not.toHaveBeenCalled();
  });
});
