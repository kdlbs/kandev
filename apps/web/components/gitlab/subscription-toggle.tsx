"use client";

import { useEffect, useRef, useState } from "react";
import { IconBell, IconBellOff } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  getIssueSubscription,
  getMRSubscription,
  setIssueSubscription,
  setMRSubscription,
} from "@/lib/api/domains/gitlab-api";
import { useToast } from "@/components/toast-provider";
import { isCurrentIdentityRequest } from "@/hooks/domains/gitlab/request-identity";

function requestIsActive(
  generation: number,
  requestIdentity: string,
  currentGeneration: { current: number },
  currentIdentity: { current: string },
) {
  return isCurrentIdentityRequest(
    generation,
    currentGeneration.current,
    requestIdentity,
    currentIdentity.current,
  );
}

export function subscriptionActionLabel(subscribed: boolean): string {
  return subscribed ? "Unsubscribe from GitLab notifications" : "Subscribe to GitLab notifications";
}

type SubscriptionIdentity = {
  kind?: "mr" | "issue";
  workspaceId: string;
  host: string;
  project: string;
  iid: number;
};

function useGitLabSubscription({
  kind = "mr",
  workspaceId,
  host,
  project,
  iid,
}: SubscriptionIdentity) {
  const [subscribed, setSubscribed] = useState(false);
  const [loading, setLoading] = useState(true);
  const { toast } = useToast();
  const requestGeneration = useRef(0);
  const identity = `${kind}\0${workspaceId}\0${host}\0${project}\0${iid}`;
  const currentIdentity = useRef(identity);
  currentIdentity.current = identity;

  useEffect(() => {
    const generation = ++requestGeneration.current;
    const requestIdentity = identity;
    setSubscribed(false);
    setLoading(true);
    const getSubscription = kind === "issue" ? getIssueSubscription : getMRSubscription;
    getSubscription({ workspaceId, host, project, iid })
      .then((state) => {
        if (requestIsActive(generation, requestIdentity, requestGeneration, currentIdentity)) {
          setSubscribed(state.subscribed);
        }
      })
      .catch((error: unknown) => {
        if (requestIsActive(generation, requestIdentity, requestGeneration, currentIdentity)) {
          toast({
            title: "Notification status unavailable",
            description: error instanceof Error ? error.message : "GitLab rejected the request.",
            variant: "error",
          });
        }
      })
      .finally(() => {
        if (requestIsActive(generation, requestIdentity, requestGeneration, currentIdentity)) {
          setLoading(false);
        }
      });
    return () => {
      if (requestGeneration.current === generation) requestGeneration.current += 1;
    };
  }, [kind, workspaceId, host, project, iid, identity, toast]);

  const toggle = async () => {
    const generation = ++requestGeneration.current;
    const requestIdentity = identity;
    const next = !subscribed;
    setLoading(true);
    try {
      const setSubscription = kind === "issue" ? setIssueSubscription : setMRSubscription;
      const state = await setSubscription({ workspaceId, host, project, iid, subscribed: next });
      if (requestIsActive(generation, requestIdentity, requestGeneration, currentIdentity)) {
        setSubscribed(state.subscribed);
        toast({
          description: state.subscribed
            ? "Subscribed to GitLab notifications"
            : "Unsubscribed from GitLab notifications",
          variant: "success",
        });
      }
    } catch (error) {
      if (requestIsActive(generation, requestIdentity, requestGeneration, currentIdentity)) {
        toast({
          title: "Notification update failed",
          description: error instanceof Error ? error.message : "GitLab rejected the action.",
          variant: "error",
        });
      }
    } finally {
      if (requestIsActive(generation, requestIdentity, requestGeneration, currentIdentity)) {
        setLoading(false);
      }
    }
  };

  return { subscribed, loading, toggle };
}

export function SubscriptionToggle(identity: SubscriptionIdentity) {
  const { subscribed, loading, toggle } = useGitLabSubscription(identity);

  const label = subscriptionActionLabel(subscribed);
  const Icon = subscribed ? IconBellOff : IconBell;
  return (
    <Button
      size="sm"
      variant="outline"
      className="h-11 cursor-pointer gap-1.5 sm:h-8"
      disabled={loading}
      aria-label={label}
      title={label}
      onClick={() => void toggle()}
    >
      <Icon className="h-4 w-4" />
      <span className="hidden lg:inline">{subscribed ? "Unsubscribe" : "Subscribe"}</span>
    </Button>
  );
}
