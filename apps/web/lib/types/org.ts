/** Organizations: the tenant boundary above users. */

export type OrgStatus = "active" | "suspended";

export type Org = {
  id: string;
  name: string;
  /** Lowercase identifier; typed verbatim to confirm deletion. */
  slug: string;
  status: OrgStatus;
  is_default: boolean;
  created_at: string;
  updated_at: string;
};

export type CurrentOrgResponse = {
  org: Org | null;
  /**
   * The instance operator tier: managing organizations themselves. It grants
   * no access inside any organization, including the operator's own.
   */
  is_operator: boolean;
};

export type ListOrgsResponse = { orgs: Org[]; total: number };
