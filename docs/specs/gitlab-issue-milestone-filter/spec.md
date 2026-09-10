---
status: draft
created: 2026-09-01
owner: tbd
issue: https://github.com/kdlbs/kandev/issues/2798
---

# GitLab issue search by milestone

## Why

The `/gitlab` page lists the connected user's GitLab issues. The only
structured narrowing it offers is a preset (assigned to me / created by me) and
a project dropdown. Every other GitLab issue attribute — state, labels,
milestone — is reachable only through the free-text **GitLab query parameters**
input in the toolbar.

That input is not a filter that composes; it is a total override.
`buildIssueSearchQuery` (`apps/backend/internal/gitlab/client_helpers.go:488`)
returns the custom query **verbatim** and discards the generated defaults
whenever it is non-empty:

```go
func buildIssueSearchQuery(filter, customQuery string) string {
	if customQuery != "" {
		return customQuery
	}
	values := url.Values{}
	values.Set("state", gitlabStateOpened)
	values.Set("scope", "all")
	...
}
```

So typing `milestone=Next` today does return milestone-filtered issues, but it
silently drops `scope=all`, and GitLab's `GET /issues` documents that `scope`
"Defaults to `created_by_me`". The user who selected the **Assigned** preset and
then typed a milestone gets issues they *authored* in that milestone — a
different set, with no indication anything changed. There is no way to express
"assigned to me **and** in milestone Next".

This feature adds milestone as a first-class filter that composes with the
selected preset instead of replacing it.

## Prior art

**Wiki leg — receipt.** Searched the Obsidian vault resolved through
`~/.obsidian-wiki/config.henry` → `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`,
QMD collection `wiki` (441 docs), via the qmd MCP server (semantic path
healthy, not the grep fallback). Five sub-queries across two calls: lex
`GitLab issue filter milestone label query integration`; vec `how should an
external issue tracker integration expose structured filters versus a raw query
escape hatch`; a hyde passage describing structured-filters-plus-escape-hatch
composition; lex `Kandev GitLab integration custom query escape hatch filter
UI`; vec `Kandev integration settings issue watches filters design decisions`.

**Result: the wiki leg returned nothing useful.** The top hits were
`synthesis/ai-sdlc-platforms-landscape.md`, `concepts/dormant-gate-anti-pattern.md`,
`concepts/guard-allowlist-ratchet-pattern.md` and `concepts/conductor-loop.md` —
all about agent orchestration and policy ratchets, none about integration query
composition or filter UI. There is no prior recorded position on structured
filters versus raw query passthrough, so nothing here departs from an existing
wiki page.

**saas-kb leg — receipt.** `search_fsm_docs` with `category: "ai_sdlc"`, three
queries: `filter issues by milestone label state in issue tracker integration`;
`GitLab issue search milestone query parameter`; `milestone cycle sprint
grouping issues`.

**Result: the saas-kb leg returned nothing useful.** Every hit scored ≤ 0.0102
(noise floor) and none described issue-filter UI: Multica architecture and
inbox pages, Factory.ai sessions/computers API reference, Warp billing FAQ and
2022 changelog, Augment Code artifact search. No vendor in the corpus documents
a milestone filter, so there is no shipped design to copy or deliberately
diverge from.

**The one source that did answer the design fork** is GitLab's own REST
documentation for `GET /issues`, read directly (see API surface). It is what
settles free-text-versus-dropdown and the special-value question below.

## What

- The `/gitlab` page's **Issues** view SHALL offer a milestone filter that
  narrows results to issues whose milestone title matches, and that composes
  with the currently selected preset rather than replacing it.
- The milestone filter SHALL be a free-text input carrying a milestone
  **title**, mirroring GitLab's own `milestone` attribute. It SHALL NOT be a
  dropdown: the page searches across every project the user can see, and
  GitLab exposes no cross-project milestone listing endpoint (milestones are
  only listed per project, `/projects/:id/milestones`, or per group). A
  dropdown would require an unbounded fan-out of upstream calls to populate.
- Filtering SHALL happen **server-side at GitLab**, so `total_count` and
  pagination stay exact. This is deliberately unlike the existing project
  dropdown, which narrows client-side (see Selection, ordering and counts).
- A milestone value SHALL be forwarded to GitLab verbatim after URL escaping,
  with exactly one normalisation: outer whitespace is trimmed (see Defaults and
  boundaries, and the trimming boundary in Client function). Beyond that trim,
  kandev SHALL NOT validate, normalise, case-fold, or interpret it.
- The **Merge requests** view SHALL NOT gain a milestone control, and
  `GET /api/v1/gitlab/user/mrs` SHALL NOT accept a `milestone` parameter. Its
  request shape and results SHALL be byte-identical before and after this
  feature.
- With no milestone entered, every request the page issues SHALL be
  byte-identical to today's.
- All new user-facing copy SHALL go through `t()` into
  `apps/web/src/locales/en/gitlab.json` and its four sibling catalogs.

## Data model

No new persistent backend state. Two shape changes:

### `gitlab.Issue` gains a milestone title

`apps/backend/internal/gitlab/models.go` `Issue`, and its mirror
`apps/web/lib/types/gitlab.ts` `Issue`:

| Field | Type | JSON | Meaning |
|---|---|---|---|
| `Milestone` | `string` | `milestone,omitempty` | Title of the issue's milestone; `""` when the issue has none |

Populated in `convertRawIssue` from a new `rawIssue.Milestone` struct field
decoded from GitLab's `milestone` object, reading its `title` only. GitLab
returns `"milestone": null` for an unassigned issue; that MUST decode to `""`,
not panic and not the string `"null"`.

This field exists for two reasons, both load-bearing: the issue row displays it
(see Presentation), and `MockClient` cannot filter on an attribute the model
does not carry (see Mock and e2e).

No other `rawIssue` milestone sub-field (`id`, `iid`, `state`, `due_date`,
`web_url`) is decoded or exposed.

### `SavedPreset` gains a milestone and the preset it was saved under

`apps/web/components/gitlab/my-gitlab/use-saved-presets.ts`:

| Field | Type | Meaning |
|---|---|---|
| `milestone` | `string` | Milestone title stored with the saved query; `""` when none |
| `preset` | `string` | Value of the sidebar preset in effect at save time; `""` when none was |

Saved presets persist server-side under the `gitlab_saved_presets` user
setting. **Presets written before this change have neither key**, and
`isSavedPreset` (`use-saved-presets.ts:42`) today tests every declared string
field with `typeof ... === "string"` (and `kind` against its two literals) and
rejects any object missing one; returning
`false` causes `readServerPresets` to `filter` the preset out entirely
(`use-saved-presets.ts:57`). Adding either field to that conjunction unchanged
would delete every saved query the user already has, on the first page load
after the upgrade, silently. Therefore, for **both** new fields identically:

- `isSavedPreset` SHALL treat a **missing** `milestone` or `preset` as valid.
- A missing value, or one present but not a `string`, SHALL be read as `""`. A
  legacy preset SHALL NOT be dropped, and SHALL behave exactly as it did before
  this feature.

#### Why `preset` is here, and why this feature is what forces it

Without it, this feature would ship a fresh instance of the exact bug it exists
to fix.

`useGitLabPageState` feeds the search hook
`preset: selection.source === "preset" ? selection.id : ""`
(`gitlab-page-client.tsx:184`). A **saved** query therefore always searches with
an empty preset, `pickFilter` returns `filter: ""`, and the request goes
upstream as `state=opened&scope=all`. Selecting a saved query silently discards
the scope it was created under — the same silent scope change described under
Why, arriving by a different route.

That is a **pre-existing** defect: it is reachable today by saving a query with
only a project filter set. But this feature makes it *newly reachable, and
newly likely*, because `canSaveCurrent` is today
`committedQuery.trim().length > 0 || projectFilter.length > 0` and SHALL gain
`|| milestone.length > 0` (see "Saving a query: when it is offered, and what
it is called", and Scenario 12). A milestone-only query becomes saveable for the
first time, and the natural way
to create one is to pick **Assigned**, type a milestone, and save. Round-tripping
that as `scope=all` would be the feature contradicting its own premise.

The cost is one optional-tolerant string field on a type this feature is already
widening, governed by the tolerance rule already written above for `milestone`.
The alternative considered and rejected was to narrow Scenario 12's claim and
record the loss under Out of scope; it was rejected because it knowingly ships
the defect the feature is about, and because the fix is smaller than the
paragraph explaining why it was skipped.

#### The effective preset, defined once and used twice

Both the search input and the save payload SHALL read the same derived value:

```
effectivePreset =
  selection.source === "preset" ? selection.id
                                : (selected saved preset)?.preset ?? ""
```

- **Search.** `useGitLabPageState` SHALL pass `preset: effectivePreset` to the
  search hook, replacing today's expression. For a sidebar preset this is
  identical to today. For a saved query it resolves to the stored value, which
  is `""` for every preset written before this feature — so **legacy saved
  queries keep exactly today's behaviour** and nothing already persisted
  changes meaning.
- **Save.** `onConfirmSave` SHALL persist `preset: effectivePreset`, so saving
  while a saved query is selected captures the scope actually in effect rather
  than resetting it to `""`. The definition is not recursive: it reads the
  *stored* value of the currently selected preset and never re-derives it from
  the value being written.

#### Boundaries on the stored preset

- **A custom query still wins.** `pickFilter` returns early when the committed
  custom query is non-empty, ignoring `preset` entirely. A saved query carrying
  both a custom query and a preset therefore searches by the custom query alone,
  exactly as today. The stored preset is not lost, and applies again if the
  custom query is later cleared.
- **An unresolvable preset degrades, it does not error.** `pickFilter` does
  `presets.find((p) => p.value === preset)` and falls back to `filter: ""`. A
  stored preset value that no longer exists in the preset list — renamed,
  removed, or saved against the other kind — therefore behaves exactly as `""`
  does: `state=opened&scope=all`, no warning, no dropped preset. The stored
  value SHALL NOT be validated against the preset list at read time, and an
  unknown value SHALL NOT cause `isSavedPreset` to reject the entry.
- **The preset list is chosen by the saved preset's `kind`.** Selecting a saved
  query already sets `selection.kind`, and the search hook is handed the preset
  list for that kind. An issue preset value is therefore never resolved against
  the MR preset list.
- **The sidebar selection does not move.** Restoring a stored preset changes
  what the *search* sends; it SHALL NOT change `selection`, which stays
  `{source: "saved", id}` so the saved query remains the highlighted row. The
  sidebar preset row SHALL NOT light up as though it had been selected
  directly.

## API surface

### Upstream: GitLab `GET /issues`

Read from GitLab's REST documentation for the list-issues endpoint, and quoted
here because three contract decisions below rest on it:

- `milestone` — "The milestone title." String. Optional.
- `milestone_id` — separate attribute supporting `None`, `Any`, `Upcoming`,
  `Started`. "`milestone` and `milestone_id` are mutually exclusive."
- `milestone` also accepts the predefined values `None` ("lists all issues with
  no milestone") and `Any` ("lists all issues that have an assigned
  milestone"), but GitLab flags those two on this attribute as slated for
  deprecation in favour of `milestone_id`.
- `scope` — "`created_by_me`, `assigned_to_me` or `all`", and it "Defaults to
  `created_by_me`."
- `state` — "Return `all` issues or just those that are `opened` or `closed`."
  No default documented.

Consequences, stated as contract:

- kandev sends `milestone`, never `milestone_id`. `milestone_id` is out of
  scope (see Out of scope).
- `None` and `Any` therefore reach GitLab and behave as GitLab defines them,
  because kandev passes the value through untouched. This is emergent, not a
  kandev feature: kandev SHALL NOT special-case them, document them in the UI,
  or promise their behaviour beyond GitLab's.
- `Upcoming` and `Started` are `milestone_id`-only. Typing either SHALL be
  forwarded as an ordinary title, matching only a milestone literally so named.
  kandev SHALL NOT translate them onto `milestone_id`.

### kandev: `GET /api/v1/gitlab/user/issues`

Handled by `httpSearchUserIssues`
(`apps/backend/internal/gitlab/controller.go:283`). One new optional query
parameter, joining the existing `workspace_id`, `filter`, `custom_query`,
`page`, `per_page`:

| Param | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `milestone` | string | No | `""` | Milestone title to narrow by |

Ordered evaluation. Each step runs only if the previous one did not return:

1. The existing `review_requested` rejection runs **first and entirely
   unchanged**, including its condition. That guard is nested inside
   `if customQuery == ""`, so it rejects with 400
   `review_requested is not supported for issues` **only when `custom_query` is
   empty**. This feature SHALL NOT widen it:
   - `filter=review_requested` with an empty `custom_query`, with or without a
     milestone, SHALL still be rejected with that 400. The milestone SHALL NOT
     be evaluated, escaped, or logged.
   - `filter=review_requested` with a **non-empty** `custom_query` SHALL keep
     today's pass-through behaviour: no 400, the request proceeds down the
     escape-hatch path below, and the milestone is folded into the custom query
     like any other. Hoisting the guard out of the `customQuery` block to make
     the rejection unconditional is **out of scope** and SHALL NOT be done here;
     it would change behaviour that exists today independently of milestone.
   Preset translation (`translateUserSearchFilter`) is nested in the same
   `customQuery == ""` block and is likewise unchanged.
2. `workspace_id` resolution (`c.workspaceClient`) runs **after** step 1, as it
   does today, and is unchanged. Its failure short-circuits before any milestone
   handling.
3. `milestone` is trimmed of leading and trailing whitespace, using the
   normative set defined under "Client function, and where trimming happens"
   (Unicode `White_Space` together with U+FEFF) rather than `strings.TrimSpace`.
4. If the trimmed value is `""`, the request proceeds exactly as today: no
   `milestone` key is added to any query string anywhere.
5. Otherwise the trimmed value is composed as below. It is escaped **exactly
   once** on both paths, but by different mechanisms, because the two paths
   carry the query in different representations.

### How the milestone reaches the query builder

The milestone travels as its **own parameter**, alongside `filter` and
`customQuery`, from the controller all the way to `buildIssueSearchQuery`. On the
preset path it is never smuggled inside either string. That requires widening the
two issue-search methods on the `Client` interface
(`apps/backend/internal/gitlab/client.go`), and this spec scopes that change in
rather than leaving the builder to discover it:

| Method | Today | After |
|---|---|---|
| `ListIssues` | `(ctx, filter, customQuery string)` | `(ctx, filter, customQuery, milestone string)` |
| `ListIssuesPaged` | `(ctx, filter, customQuery string, page, perPage int)` | `(ctx, filter, customQuery, milestone string, page, perPage int)` |

`buildIssueSearchQuery` takes the same third string
(`buildIssueSearchQuery(filter, customQuery, milestone string) string`), and
`PATClient.ListIssuesPaged` — its only caller — forwards what it was handed. The
`Service` wrappers `SearchUserIssues` and `SearchUserIssuesPaged`
(`service_search.go`) gain the parameter and pass it through unchanged.

**A positional parameter, not an options struct.** The package's search methods
are uniformly positional, and `SearchMRsPaged(ctx, filter, customQuery, page,
perPage)` is deliberately the mirror image of `ListIssuesPaged`. Introducing an
options struct for issues alone would break that symmetry for the sake of one
added field; the single options struct in the package
(`TaskMRAutomationOptions`) belongs to an unrelated domain and is not a general
convention. The accepted cost is three adjacent string parameters, which a
transposed call would compile.

Two properties bound that risk, and neither is "the call sites are all listed":

1. **The arity change is the guard.** `milestone` is appended **after**
   `customQuery`, so `filter` and `customQuery` keep their existing positions
   and the arity goes 3 to 4 and 5 to 6. No existing call compiles unchanged.
   The compiler therefore visits every call site in the repository, and a
   silently reinterpreted *untouched* call is impossible. The residual risk is a
   transposition introduced while editing a call site or writing a new one — a
   normal review risk, not a hidden one.
2. **The blast radius is one package.** Every caller of these two methods is
   inside `apps/backend/internal/gitlab/`, verified by search. Production call
   sites are enumerated under Files touched. Test sites are **not** enumerated
   there and are not zero; there are five, and the implementer will meet every
   one as a compile error. Three are calls —
   `pat_client_search_actions_test.go:57`, `pat_client_test.go:433` and
   `pat_client_test.go:470` — and two are fake clients that *implement* the
   interface: `service_issue_query_test.go:16`
   (`captureIssueQueryClient.ListIssues`) and `service_mentions_test.go:19`
   (`mentionRecordingClient.ListIssuesPaged`). The fakes are implementations
   rather than calls, so they must be updated for the package to build at all;
   updating only the three calls leaves the package uncompilable.

**Scope guard.** `apps/backend/internal/github` declares its **own** `Client`
interface with identically named `ListIssues` and `ListIssuesPaged` methods
carrying today's signatures. It is a different interface in a different package
and SHALL NOT change. GitHub gains no milestone filter (see Out of scope), and
the name collision SHALL NOT be read as a shared contract.

**Callers with no milestone pass `""`** and keep today's behaviour exactly. That
includes `service_issue_watches.go`, whose feature-level exclusion is unaffected:
issue watches gain no milestone capability, no DB column, and no dialog field.
Updating that call site is a mechanical consequence of the signature change, not
a feature, and the two SHALL NOT be conflated.

Composing the milestone into the generated query rather than into the custom
query follows the `Client` interface's own `filter`-versus-`customQuery` split,
which has the same meaning on every method that takes the pair. Cited precisely,
because the exact wording differs by method:

- `ListReviewRequestedMRs` (`apps/backend/internal/gitlab/client.go:37`)
  documents `filter` as "an optional additional GitLab API filter (e.g.
  `project_id=123` or `milestone=v1`)" and `customQuery` as the thing that
  "replaces the entire generated query". **`milestone=v1` is the interface's own
  example of a `filter` value — but it appears on this MR method**, whose
  milestone support this spec puts out of scope.
- `ListIssues` (`apps/backend/internal/gitlab/client.go:107`) documents the same
  pair more tersely — "filter is an optional additional API filter; customQuery,
  when non-empty, replaces the entire generated query" — and does **not** name
  milestone.

So the precedent is the shared semantics of the pair, not a milestone example on
the issue method. A builder MAY align `ListIssues`'s doc comment with its
sibling's wording; nothing in this spec depends on that.

Composition, by whether `custom_query` is present:

- **`custom_query` empty** (preset path). The milestone is set as a
  **structured key on the `url.Values` that `buildIssueSearchQuery` already
  builds**, after the defaults and after `appendFilter` has merged the
  preset-translated filter:

  1. `state=opened` and `scope=all` are set as today.
  2. `appendFilter(values, filter)` merges the preset-translated filter over
     them, exactly as today.
  3. `values.Set("milestone", <trimmed value>)` — the raw trimmed title, not a
     pre-escaped one. `values.Encode()` performs the single escape.

  The milestone SHALL NOT be string-concatenated onto the `filter` argument.
  This ordering is load-bearing twice over:

  - **Appending before preset translation would lose the preset.**
    `translateUserSearchFilter` returns `""` for any token containing `=` or
    `&`, so `assigned_to_me&milestone=Next` would translate to nothing and
    `scope=assigned_to_me` would silently vanish — the exact bug this feature
    exists to fix.
  - **Appending into the filter string at all would let the milestone be
    silently dropped.** `appendFilter` does `url.ParseQuery(filter)` and returns
    early on error; its own doc comment says "Unparseable filters are ignored".
    A filter that fails to parse (for example `filter=%zz`, which
    `translateUserSearchFilter` passes through untouched because it contains no
    `=` or `&`) would take the concatenated milestone down with it, and the user
    would get an unfiltered `state=opened&scope=all` listing with no indication
    the milestone was discarded. Setting the key **after** `appendFilter` makes
    the milestone structurally immune to that branch. See the Failure modes row
    for an unparseable `filter`.

  For the Assigned preset and milestone `Next`, the query GitLab receives is
  `scope=assigned_to_me&state=opened&milestone=Next` (parameter order is
  whatever `url.Values.Encode` produces; only the set of key/value pairs is
  specified).
- **`custom_query` non-empty** (escape-hatch path). The milestone is folded
  into the custom query under these rules, in order:
  1. Parse `custom_query` with `url.ParseQuery`.
  2. If parsing **succeeds** and the result already contains a `milestone` key
     (with any value, including empty), the custom query is used **unchanged**
     and the `milestone` parameter is ignored. The custom query wins.
  3. Otherwise (parse succeeded without a `milestone` key, **or** parsing
     failed) append `&milestone=<escaped>` before any `#` fragment. Malformed
     values are rejected; direct builders also keep defensive append.
  This mirrors the established `appendLabelsToQuery` rule
  (`apps/backend/internal/gitlab/service_watches.go:840`), including its
  parse-failure branch, so the two folds behave identically.
- The escape-hatch path SHALL NOT re-introduce `scope=all` or `state=opened`.
  Today's "custom query replaces the generated defaults" semantics are
  preserved unchanged; this feature narrows nothing else and widens nothing.

Escaping is a correctness **and** injection boundary. `milestone` arrives
already URL-decoded by the HTTP layer. A title containing `&`, `=`, `#`, `%`,
`+`, or a space MUST NOT introduce, remove, or alter any other query parameter
seen by GitLab. Escaping SHALL be applied **exactly once on each path** — a
value escaped twice (`Q3+%2F+Q4` becoming `Q3%2B%252F%2BQ4`) is a defect, and so
is a value not escaped at all.

The mechanism differs by path because the two paths hold the query differently,
and the builder SHALL NOT unify them:

| Path | Representation | Who escapes | Input to it |
|---|---|---|---|
| Preset (`custom_query` empty) | `url.Values` | `values.Encode()` | the **raw trimmed** title |
| Escape hatch (`custom_query` non-empty) | opaque string returned verbatim | `url.QueryEscape` | the **raw trimmed** title |

Passing an already-escaped value to `values.Set` would double-escape it; passing
a raw value into the custom-query concatenation would under-escape it. Each path
takes the raw trimmed title and escapes once.

Response shape is unchanged apart from the new optional `milestone` field on
each issue. Malformed values return HTTP 400 before lookup; upstream errors
continue through
`writeWorkspaceClientActionError(ctx, err, "issue search")`.

### Client function, and where trimming happens

**The trimming boundary is the commit, and it is normative.** The milestone has
a draft value and a committed value (Scenario 7, mirroring the custom-query
input). The **commit** trims: when a draft is committed by Enter or by blur, the
value written to the committed state SHALL be the draft with leading and
trailing whitespace removed, "whitespace" meaning the set defined immediately
below. Every consumer downstream therefore sees an already-trimmed string, and
there is exactly one place in the frontend that trims.

**What "whitespace" means: one normative set, stated explicitly.** Neither
language's default trim is the contract, because they are not the same
function. Go's `strings.TrimSpace` uses `unicode.IsSpace`, which is the Unicode
`White_Space` property; JavaScript's `String.prototype.trim` removes ECMAScript
*WhiteSpace* + *LineTerminator*. Those two sets differ, and they differ in
**both** directions, so "both sides trim" is not a specification:

| code point | `strings.TrimSpace` alone | `String.prototype.trim` alone |
|---|---|---|
| U+0085 (NEL) | trimmed | **kept** |
| U+FEFF (BOM) | **kept** | trimmed |

The normative set for this feature is **Unicode `White_Space` together with
U+FEFF**. Trimming means removing leading and trailing runes in that set, and
nothing else. Neither side SHALL call its language default; each SHALL
implement that set:

| Side | Implementation |
|---|---|
| Go (controller) | `strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) \|\| r == '\uFEFF' })` |
| TypeScript (commit) | `s.replace(/^[\s\u0085]+\|[\s\u0085]+$/gu, "")` |

`\s` in a JavaScript regular expression already covers ECMAScript *WhiteSpace*
+ *LineTerminator*, which includes U+FEFF; adding `\u0085` explicitly makes the
class exactly `White_Space` together with U+FEFF, matching the Go side. Each
expression is one line, and each SHALL live behind a single named helper on its
own side so the set is written once per language rather than re-derived at each
call site.

These two expressions were measured against U+0009, U+000A, U+000B, U+000C,
U+000D, U+0020, U+0085, U+00A0, U+1680, U+2000, U+2028, U+2029, U+202F, U+205F,
U+3000, U+FEFF, U+200B, U+180E, U+2060 and U+00B7. They agree on every one: the
first sixteen are trimmed, the last four are preserved. That agreement is a
property a test SHALL assert on each side (Scenario 27), not an assumption the
reader is asked to take on faith.

**Characters deliberately NOT trimmed**, because they are whitespace under
neither definition and a milestone title may legitimately contain them: U+200B
(zero-width space), U+2060 (word joiner), and U+180E (Mongolian vowel
separator, category `Cf` since Unicode 6.3). A title consisting only of one of
these is non-empty, is sent, and matches only a GitLab milestone literally so
named.

**How that is achieved, because the shared hook does not trim by itself.**
`useCommittedQuery` exposes `commit()`, which takes no argument and copies the
draft into the committed value verbatim. It does not trim, and it SHALL NOT be
taught to: the custom-query input shares that hook and its behaviour is out of
scope. The milestone input SHALL therefore commit by calling
`setImmediate(trimGitLabMilestone(draft))` rather than `commit()`. `setImmediate` writes the
draft and the committed value together, which gives both halves of the rule:

- the committed value is trimmed at the instant of commit, as required above; and
- the visible input is normalised to the same trimmed text on Enter or blur.
  That is intended, not a side effect. An input still reading `"  Next  "` while
  the committed value is `"Next"` invites a second, pointless commit.

Trimming SHALL happen only on commit, never per keystroke: trimming the draft on
every change would make a leading space impossible to type through.

**Every change to the committed milestone resets pagination, in the same
handler.** The rule is stated over the value, not over one gesture, because the
committed milestone changes on four paths and only one of them is a commit:
pressing Enter or blurring the input; selecting a saved query (restore);
selecting a sidebar preset (clear); and deleting the selected saved query
(clear). Each SHALL call the search hook's `setPage(1)` from inside the same
event handler that changes the milestone, so React batches both state updates
into one render.

The three non-commit paths need saying because the existing page-reset effect
does not cover them. Its dependencies are `[preset, customQuery, kind]`, and a
saved query can differ from the current state **only** in its milestone — same
effective preset, same empty custom query, same kind — in which case the effect
never fires, the page stays where it was, and a milestone-narrowed fetch is
issued for page 3 of a result set that now has one page. The same holds for the
delete path when the fallback preset happens to equal the effective preset
already in force. Resetting in the handler is uniform across all four paths and
does not depend on which of them the effect would have caught anyway.

The mechanism is specified because the obvious alternative is wrong.
`useGitLabSearch` already resets the page from an effect whose dependencies are
`[preset, customQuery, kind]`. Adding `milestone` to that array would satisfy
Scenario 6 and break Scenario 7: the reset effect and the fetch effect run in the
same commit phase, so the fetch would fire once for the **previous** page
carrying the **new** milestone, and only then fire again for page 1. `requestSeq`
discards the stale response, but two requests would have left the browser, and
Scenario 7 requires exactly one. Committing and resetting together yields one
render and therefore one request, whatever page the user was on.

The existing `customQuery` entry in that dependency array is left exactly as it
is. Its double-fetch predates this feature, fixing it is out of scope, and the
resulting asymmetry is deliberate: the milestone SHALL NOT be moved into that
effect for consistency's sake.

**The commit also invalidates the accumulated project list.** The project
dropdown is not populated from an endpoint; `useKnownProjects` accumulates the
`project_path` values seen across pages and resets only when its `resetKey`
changes. That key is built at `gitlab-page-client.tsx:147` from four segments —
`selection.kind`, `selection.source`, `selection.id`, and the trimmed committed
query — and the milestone is not among them. Left alone, narrowing to a
milestone would keep offering every project the user had browsed before,
including ones with no issue in that milestone; selecting one would return an
empty list from a dropdown that claimed to list what is there.

The **committed** milestone SHALL therefore be appended as a fifth segment of
`resetKey`. The draft SHALL NOT be, or the list would reset on every keystroke.
The segments are joined with `:` and a milestone title may itself contain `:`,
which is harmless here: `resetKey` is only ever compared for equality and is
never parsed back into segments, so a colon cannot make two distinct states
collide — it can only ever be part of one state's identity.

**That alone is not enough, and the spec says so rather than leaving the gap for
the implementer to discover.** `recordForKey` clears the accumulator when the key
changes and then records whatever it was handed **in that same call**
(`use-known-projects.ts:30-46`). On the render where the milestone commits, the
key is already new but `search.rawItems` still holds the **previous** result set,
because `fetchData` sets `loading: true` while spreading the old state and
therefore keeps the old items on screen (`use-gitlab-search.ts:99-100`). The
effect would clear the set and immediately refill it with the stale projects
under the new key; when the real results arrived, `recordForKey` would only
**add** to them, since the key had stopped changing. The reset would have
accomplished nothing.

So `useProjectOptions` SHALL pass an **empty** project list while a search is in
flight, deriving `pageProjects` from `search.rawItems` only when
`search.loading` is false. That yields the right behaviour in both directions:
on a key change the clear lands with nothing to refill and the new results
populate a genuinely empty set; on ordinary pagination, where the key does not
change, the in-flight empty list clears nothing and the next page's projects are
still accumulated on top of the previous ones.

This also repairs the identical staleness for `committedQuery`, which shares the
key and the effect. That is a consequence, not a second feature: there is one
accumulator and one call site, and the milestone cannot be made correct while
leaving the neighbouring segment broken. It is distinct from, and does not
touch, the `customQuery` double-fetch disclaimed above.

`useProjectOptions` takes four positional parameters today and would take six.
Six exceeds the `max-params` cap of 5 (`apps/web/eslint.config.mjs:47`, warn,
the same warn-level family as the `max-lines` cap cited under Constraints), so it
SHALL take a single options object instead:
`{ selection, committedQuery, milestone, items, loading, projectFilter }`. The
signature change is free — the function is being moved into
`use-gitlab-page-state.ts` by the extraction specified under Constraints, and it
has exactly one call site. (Scenario 29.)

**Deleting the selected saved query clears the milestone.** `onDeleteSaved`
already resets `customQuery` and `projectFilter` when the deleted query is the
selected one, and it SHALL reset the milestone — draft and committed, both to
`""` — in that same branch and the same batch. Without it, deleting a saved
query whose milestone was `Next` would leave the milestone filter applied with
nothing on screen explaining why the list is narrow.

Deleting a saved query that is **not** the selected one SHALL change no filter
state and SHALL trigger no refetch, exactly as today. (Scenario 30.)

That single rule settles three things that would otherwise disagree:

- `searchUserIssues` (`apps/web/lib/api/domains/gitlab-api.ts:194`) accepts an
  optional `milestone` and SHALL set the `milestone` search param **only** when
  the value is a non-empty string — matching how it already treats `filter` and
  `customQuery`, and requiring no trim of its own. Because the committed value
  is already trimmed, a whitespace-only entry is `""` by the time it reaches
  here, the param is omitted, and the outgoing
  `/api/v1/gitlab/user/issues` URL is byte-identical to today's. **This is what
  makes Scenario 3 true.** Had trimming lived only in the controller, a
  whitespace-only entry would have produced a frontend URL carrying
  `milestone=++` and Scenario 3 would be false.
- The value persisted into a `SavedPreset` is the **committed (trimmed)** value,
  never the raw draft. Two saved queries whose titles differ only in outer
  whitespace are therefore the same query, and a saved preset always reproduces
  the result set that was on screen when it was saved (Scenario 12).
- The controller **also** trims (API surface, step 3), applying the same
  normative set. That is not redundant: it is the boundary for direct API
  callers, who never pass through the frontend. The two trims agree **because
  each implements the set above**, not because the two languages happen to
  agree — they do not, and the earlier draft of this spec was wrong to say they
  did. Given that shared set, a value that survives one survives the other
  unchanged, and trimming an already-trimmed string is a no-op, so the double
  application is idempotent rather than a double-normalisation.

A request with no milestone SHALL produce a byte-identical URL to today's.

## Selection, ordering and counts

- **Result ordering is GitLab's and is not changed.** The milestone parameter
  SHALL NOT introduce a sort, an `order_by`, or a re-sort of the returned page.
  Two requests differing only in milestone SHALL preserve upstream order within
  their own result sets. kandev states no tiebreak for the live path because it
  imposes no ordering there.
- **The badge count.** `displayedCount` in
  `apps/web/app/gitlab/gitlab-page-client.tsx` currently shows
  `search.items.length` when `projectFilter` is set and `search.total`
  otherwise, because the project dropdown narrows client-side and would
  otherwise read "47" beside three rows. The milestone filter narrows
  **server-side**, so `search.total` is already the milestone-narrowed total.
  The milestone SHALL NOT be added to that condition: with a milestone set and
  no project filter, the badge SHALL show `search.total`. With both set, the
  existing project-filter branch continues to win and the badge shows
  `search.items.length`.
- **Pagination.** `search.total` reflects the milestone-narrowed count, so page
  count is exact. Committing a milestone change SHALL reset the view to page 1
  (see Scenario 6).

## Mock and e2e

`MockClient` is selected by `newClient` when the environment variable
**`KANDEV_MOCK_GITLAB`** is `"true"` (`apps/backend/internal/gitlab/factory.go:37`).
It is not selected by `KANDEV_E2E_MOCK`. E2E runs get the mock only because the
`e2e` profile in `profiles.yaml` sets `KANDEV_MOCK_GITLAB: e2e: "true"` as a
separate entry alongside `KANDEV_E2E_MOCK`. Anyone running the mock outside that
profile SHALL set `KANDEV_MOCK_GITLAB` explicitly; setting `KANDEV_E2E_MOCK`
alone does not produce a `MockClient`.

Today the mock has **three** deficiencies that block a demonstration of this
feature. `MockClient.ListIssues` **ignores both `filter` and `customQuery`
entirely** and returns every seeded issue in Go map-iteration order; and
`MockClient.ListIssuesPaged` — which is the method the controller actually calls,
delegating to `ListIssues` — **ignores `page` and `perPage` entirely**, returning
the whole list with `TotalCount: len(issues)` for every requested page. All three
are in scope for the mock path only:

- `MockClient` receives the same three search inputs as any other client
  (`filter`, `customQuery`, `milestone`) and receives them **as the controller
  passed them**: it never calls `buildIssueSearchQuery`, so it sees the raw
  `custom_query` from the request, not a folded one. It SHALL resolve an
  **effective milestone** from those three, by the first rule that applies:
  1. if `customQuery` parses and carries a `milestone` key, the effective
     milestone is that key's **first** value, **including when that value is
     empty**. First, not last and not all of them: `url.Values.Get` returns the
     first, and the mock SHALL use `Get` so that a hand-written
     `milestone=Old&milestone=Next` picks `Old` (Scenario 31);
  2. otherwise, if the `milestone` parameter is non-empty, it is the effective
     milestone;
  3. otherwise, if `filter` parses and carries a non-empty `milestone` key, its
     **first** value is the effective milestone, on the same `Get` rule —
     retained only so a Go caller that hand-builds a filter string still behaves
     sensibly; the controller never puts one there;
  4. otherwise the effective milestone is empty.
  A `customQuery` carrying a `milestone` key therefore wins over the parameter,
  reproducing the controller's fold precedence (Scenario 4) without the mock
  having to replicate the fold itself. Rule 1 deliberately lets an **empty**
  key win rather than falling through: on the live path an empty key likewise
  suppresses the fold and reaches GitLab as an empty `milestone` (Scenario 21),
  so a mock that fell through to the parameter would filter where GitLab would
  not.
- With a non-empty effective milestone, `ListIssues` SHALL return only seeded
  issues whose `Milestone` equals it, compared as an exact case-sensitive
  string. With an empty one, every seeded issue is returned, as today.
- `MockClient.ListIssuesPaged` SHALL forward the milestone to `ListIssues`
  rather than resolving it a second time, so the two can never disagree.
- `MockClient.ListIssues` SHALL return issues in a **deterministic order**:
  `ProjectPath` ascending, then `IID` ascending. This is a named tiebreak on
  named fields; the pair is unique because `MockClient` keys its issue map on
  exactly `{Project, IID}`. Without it a multi-issue assertion is flaky by
  construction.
- `MockClient.ListIssuesPaged` SHALL page over the filtered, sorted result.
  Given the ordered list produced above, it SHALL return the slice starting at
  `(page - 1) * perPage` of length at most `perPage`, and SHALL set
  `TotalCount` to the length of the **whole filtered list**, not the length of
  the returned slice. A `page` beyond the end yields an empty `Issues` slice
  with the same `TotalCount`; `page` and `perPage` are echoed back as given.
  This is scoped in rather than excluded because the spec asserts an exact badge
  count (Scenario 9) and a page reset (Scenario 6), and neither is demonstrable
  against a mock that returns every issue for every page.
- **Paging bounds are defined, so the mock cannot panic.** Over HTTP,
  `paginationFromQuery` already clamps `page` to `>= 1` and `perPage` to `> 0`,
  so the degenerate values below are unreachable from the controller. But
  `ListIssuesPaged` is also called directly from Go tests, where nothing clamps
  first, and a naive `(page - 1) * perPage` slice would panic on a negative
  index. `MockClient.ListIssuesPaged` SHALL therefore treat `page < 1` as `1`
  and `perPage < 1` as "return no issues", computing the start offset only
  after that normalisation, and SHALL clamp the end offset to the length of the
  filtered list. `TotalCount` SHALL be the full filtered count in every one of
  these cases, and the `Page` and `PerPage` echoed back SHALL be the normalised
  values actually used, not the raw arguments — so a caller can always tell
  which window it received.
- Every filter key other than `milestone` SHALL continue to be ignored by the
  mock. This feature does not teach it `state`, `scope`, `labels`, or
  `assignee_username`.
- `POST /mock/issues` gains no new field of its own: it already decodes
  `[]Issue`, so `milestone` becomes seedable the moment the model carries it.
- These are properties of `MockClient` only. `PATClient` SHALL NOT gain any
  local filtering or sorting; GitLab remains the only thing that filters real
  results.

## Presentation

- The milestone control renders in the toolbar only when the Issues view is
  selected. In the Merge requests view it SHALL NOT render **at all** — not
  hidden by a CSS class, not rendered disabled. The MR view's toolbar DOM is
  byte-for-byte what it is today (Scenario 10).
- It is a single-line text input, sitting alongside the existing project
  dropdown inside `ListToolbar`'s `filter` slot. The shared
  `IntegrationListToolbar` (used by GitHub and Linear) SHALL NOT change its
  props or markup; GitLab composes both controls into the one existing
  `filter` node.

**`ListToolbar`'s new props, specified because there is only one call site.**
`ListToolbar` is rendered once (`gitlab-page-client.tsx:301`) for both views, so
the Issues-only rule above cannot be expressed by rendering a different
component; it is a prop. Five props are added, and all five are **REQUIRED** —
optional props with defaults would let the MR view silently acquire a milestone
input if a future call site forgot one:

| Prop | Type | Meaning |
|---|---|---|
| `showMilestoneFilter` | `boolean` | The page passes `selection.kind === "issue"`. When `false`, the input subtree is not rendered. |
| `milestone` | `string` | The **draft** value; what the input displays. |
| `committedMilestone` | `string` | The committed value. Used **only** to decide whether a blur commits, mirroring how `committedQuery` is used for the custom-query input. |
| `onMilestoneChange` | `(value: string) => void` | Draft setter, fired per keystroke. |
| `onCommitMilestone` | `() => void` | Promotes draft to committed. |

The milestone input implements Enter-to-commit and blur-to-commit itself, on the
same terms as the custom-query input (Scenario 7). `committedMilestone` exists
for the same single reason `committedQuery` does in
`integration-list-toolbar.tsx`: to compute dirtiness. The shared toolbar's
existing "press Enter" hint is driven by `customQuery !== committedQuery` and
SHALL remain scoped to the custom-query input; the milestone input SHALL NOT
render a hint of its own. Adding a second hint would mean two competing
affordances in one toolbar row for what is, to the user, one habit.

- Its placeholder is an example milestone title and stays an untranslated
  literal: a translated example stops being a usable example. Because a
  placeholder is a **JSX attribute**, it is seen by `i18next/no-literal-string`
  — which is jsx-only and an error tree-wide — so the marker SHALL be
  `// eslint-disable-next-line i18next/no-literal-string -- example milestone title`,
  following `components/gitlab/watch-dialog.tsx:132,148,173`. It SHALL NOT be
  `// i18n-exempt:`, which addresses a different tool
  (`scripts/check-nonjsx-copy.mjs`, covering positions the eslint rule cannot
  see) and would leave the eslint error standing and CI red. The two markers are
  not interchangeable; see Saving a query for the contrasting case where
  `// i18n-exempt:` is the correct one. Its visible label and accessible name go
  through `t()`. Note that the nearest neighbour in this same toolbar,
  `list-toolbar.tsx:60`, is `t("gitlab:allProjects")` — a translated
  placeholder — so the exemption rests on the example-value argument alone and
  not on any local precedent.
- Test id: `gitlab-milestone-filter`.
- An issue row SHALL render its milestone title as a plain text chip in the
  same chip row as the existing labels (`issue-list.tsx`) when `milestone` is
  non-empty, and render nothing there when it is empty. The chip is text only:
  no link, no icon, no click behaviour, no interaction with the milestone
  filter.
- **Order is specified, and it is specified against the whole row, not just
  against the chips.** That row is not chips alone: in `issue-list.tsx` the same
  flex container holds, in order, `project_path#iid`, a separator, the
  "by author ... ago" text, and only then the label chips rendered by
  `IssueLabels`. The milestone chip SHALL be inserted **immediately before the
  first label chip** — after the existing author and time metadata, not at the
  head of the row. It leads the chips; it does not displace the row's
  identifying text. Within the chips the single higher-level grouping leads,
  since a row has at most one milestone and may have many labels.
- The milestone chip SHALL carry its own stable test id,
  `gitlab-issue-milestone`, distinct from the label chips', so that this
  position is assertable in a test rather than eyeballed: the requirement is
  "identifiable in the DOM", not "looks different".
- The existing `labels.slice(0, 4)` cap on rendered label chips is unchanged,
  and the milestone chip SHALL NOT count against it: a row with a milestone and
  four labels renders five chips.

## Failure modes

| Condition | Behaviour |
|---|---|
| Milestone empty or whitespace-only | Treated as absent. No `milestone` key sent. Identical to today. |
| Milestone matches no issues | Empty result set, `total_count` 0, existing empty state rendered. Not an error. |
| Milestone set while `filter=review_requested` | Existing 400 `review_requested is not supported for issues`, unchanged. Milestone never evaluated. |
| `custom_query` already contains `milestone` | Custom query wins; the `milestone` parameter is silently ignored. No error, no warning. |
| `custom_query` unparseable by `url.ParseQuery` | HTTP 400; no GitLab call. |
| `filter` unparseable by `url.ParseQuery` (e.g. `filter=%zz`) | Today `appendFilter` returns early and the whole filter is silently dropped. That is unchanged and out of scope. The **milestone still applies**, because it is set on `url.Values` after `appendFilter` returns rather than concatenated into the `filter` string. A malformed filter therefore degrades to "no preset, milestone honoured", never to "milestone silently lost". |
| Milestone title contains `&`, `=`, `#`, `%`, `+`, or spaces | Escaped once; reaches GitLab as one parameter value. No other parameter is added or altered. |
| GitLab returns an error | Existing `writeWorkspaceClientActionError` path, unchanged. |
| GitLab returns `"milestone": null` on an issue | Decodes to `""`. No chip rendered. |
| GitLab is not configured / not connected | Existing connect notice; no search fires. Unchanged. |
| Legacy saved preset without a `milestone` key | Loads normally with `milestone: ""`. Never dropped. |

## Persistence guarantees

- The milestone filter is **not** persisted as page state. Reloading `/gitlab`
  restores the default selection with an empty milestone, exactly as the
  project filter and custom query behave today.
- A milestone is persisted only when the user explicitly saves a query, as part
  of that `SavedPreset`, through the existing `gitlab_saved_presets` user
  setting and its existing `createQueuedUserSettingsSync` writer.
- Saved-preset writes remain last-write-wins across concurrent tabs, unchanged.
  This feature adds a field to the persisted object; it does not add
  reconciliation, merging, or conflict detection.

## Idempotency, retry and concurrency

- `GET /api/v1/gitlab/user/issues` is read-only. Repeating a request with the
  same parameters is safe and SHALL produce the same upstream query string
  byte-for-byte. There is no request id, no dedup, and no server-side caching.
- The custom-query fold is idempotent **when the custom query parses**:
  applying it to a query that already carries `milestone` returns that query
  unchanged. When the custom query does **not** parse, the fold is append-only
  by design — it mirrors `appendLabelsToQuery`, whose parse-failure branch
  appends unconditionally — so the "already has the key" guard can never fire
  and a second application would append a second `&milestone=`. That is
  unreachable in practice, and structurally rather than luckily so: every
  request re-folds from the raw `custom_query` it was handed, and a folded query
  is never fed back in as input. A retry therefore cannot accumulate
  `&milestone=` twice on either branch.
- Two callers reading the same workspace concurrently do not interact; the
  handler holds no per-request mutable state.
- **Two callers, same row.** The only row this feature writes is the
  `gitlab_saved_presets` user setting, and it writes it only on an explicit
  save or delete. Two tabs saving concurrently resolve last-write-wins: the
  second write replaces the whole preset array, so a preset created in the
  first tab and not yet visible to the second is lost. This is exactly today's
  behaviour for `customQuery` and `projectFilter`, and this feature SHALL NOT
  change it. Adding `milestone` to the object does not introduce a new
  conflict surface, and no merge, version check, or reconciliation is added.
  Search itself is read-only and has no row to contend for.
- **In-flight ordering in the browser.** `useGitLabSearch` already discards
  stale responses through its `requestSeq` counter. The milestone SHALL flow
  through that same `fetchData` path, so a slow response for an earlier
  milestone value can never overwrite a newer one. It SHALL NOT be applied by a
  second, parallel fetch.
- **Mock concurrency.** `MockClient.ListIssues` filters and sorts while holding
  the existing `c.mu` lock, so a concurrent `SeedIssue` cannot observe or
  produce a torn slice.

## Defaults and boundaries

- **Default.** The milestone is `""` everywhere it appears: the API parameter
  when omitted, the toolbar input on first render, a legacy `SavedPreset`, and
  an issue with no milestone. `""` always means "do not filter" on the request
  side and "render nothing" on the display side. There is no sentinel value and
  no null.
- **Whitespace.** Leading and trailing whitespace is trimmed before the
  emptiness test and before escaping, so `"  "` is absent and `" Next "`
  filters on `Next`. **"Whitespace" here means one specific set: Unicode
  `White_Space` together with U+FEFF**, defined normatively under "Client
  function, and where trimming happens". It is deliberately neither
  `strings.TrimSpace` nor `String.prototype.trim`, which disagree with each
  other in both directions; both sides implement the stated set instead.
  Interior whitespace is preserved: `Q1 Planning` is one title, not two tokens.
  The trim happens **at commit** in the frontend and **again** in the controller
  for direct API callers. Because the frontend commits a trimmed value, the
  client function never sees a whitespace-only string and omits the parameter
  outright — which is what makes the URL in Scenario 3 byte-identical rather
  than merely equivalent. **Because both implement the same set**, trimming an
  already-trimmed value is a no-op and the two trims cannot disagree; that
  conclusion follows from the shared set, not from the two languages' defaults.
- **Case.** Not folded. Matching is GitLab's, which treats milestone titles as
  case-sensitive; kandev SHALL NOT lower-case, upper-case, or otherwise
  normalise the value. The `MockClient` comparison is likewise exact and
  case-sensitive, so the mock and the live path agree.
- **Length.** No minimum and no maximum is imposed by kandev. A title longer
  than GitLab accepts produces GitLab's own response, surfaced through the
  existing error path. There is no client-side `maxLength` on the input.
- **Character set.** Any Unicode string is accepted, including one that is
  entirely punctuation or entirely non-Latin. The only transformation applied
  is a single `url.QueryEscape`.
- **Multiplicity.** Exactly one milestone; no attempt is made to OR several
  titles together. A repeated key resolves to the **first** value at every place
  kandev has to choose, because every such reader is `url.Values.Get`, which
  returns the first. That rule covers three distinct places, and they agree:
  1. a repeated `milestone` parameter on the kandev HTTP request (controller);
  2. a repeated `milestone` key inside `custom_query`, when the controller's
     fold inspects it to decide whether to append (see How the milestone reaches
     the query builder);
  3. a repeated `milestone` key inside `custom_query` or `filter` when
     `MockClient` resolves its effective milestone.

  Only the mock ever *filters* on the chosen value. On the live path the
  non-empty custom query is forwarded upstream **unchanged**, both pairs
  included, and GitLab applies its own resolution — kandev inspects the
  duplicate to decide whether to fold, and then does not have to choose
  (Scenario 31).
- **Page size and pagination bounds.** Unchanged. `SEARCH_PAGE_SIZE` stays 25,
  and the milestone does not alter `page` or `per_page` handling beyond the
  reset to page 1 on commit.

## Scenarios

1. **Milestone composes with a preset.** GIVEN the Issues view with the
   **Assigned** preset selected and no custom query, WHEN the user commits the
   milestone `Next`, THEN the request to GitLab carries the pairs
   `scope=assigned_to_me`, `state=opened` and `milestone=Next`, and the list
   shows only issues assigned to the user in milestone `Next`.

2. **Milestone does not silently reset scope.** GIVEN the same state as
   Scenario 1, WHEN the milestone is committed, THEN `scope` SHALL NOT be
   absent from the upstream query and SHALL NOT be `created_by_me`.

3. **Empty milestone changes nothing.** GIVEN the Issues view with any preset,
   WHEN the milestone input is empty, or contains only whitespace **and has
   been committed**, THEN the committed value is `""`, the client omits the
   `milestone` search param entirely, and the outgoing
   `/api/v1/gitlab/user/issues` URL is byte-identical to the one produced
   before this feature — as is the upstream GitLab query.

   **And at the other boundary.** GIVEN a caller that bypasses the frontend and
   requests `/api/v1/gitlab/user/issues?milestone=%20%20`, WHEN the controller
   trims, THEN the value is empty, no `milestone` pair is added to the upstream
   query, and the response is identical to the same request with the parameter
   omitted.

4. **Custom query wins.** GIVEN a custom query of
   `state=closed&milestone=Old`, WHEN the user also commits the milestone
   `Next`, THEN the upstream query is exactly `state=closed&milestone=Old` and
   no second `milestone` pair is present.

5. **Custom query without a milestone is extended.** GIVEN a custom query of
   `state=closed`, WHEN the user commits the milestone `Next`, THEN the
   upstream query carries both `state=closed` and `milestone=Next`, and SHALL
   NOT carry `scope=all`.

6. **Any change to the committed milestone resets pagination.** GIVEN the user
   is on page 3 of an unfiltered issue list, WHEN a milestone is committed, THEN
   the view requests page 1 and the pagination control reports page 1. GIVEN the
   user is on page 3 and selects a saved query that differs from the current
   state **only** in its stored `milestone` — same effective preset, same empty
   custom query, same kind, so the existing `[preset, customQuery, kind]` reset
   effect does not fire — THEN the view still requests page 1. GIVEN the user is
   on page 3 with a committed milestone and selects a sidebar preset, or deletes
   the selected saved query, THEN the milestone clears and the view requests
   page 1 in the same render.

7. **Commit semantics match the custom-query input.** GIVEN the milestone input
   has focus, WHEN the user types without pressing Enter, THEN no request is
   issued; WHEN the user presses Enter, or blurs the input while its draft
   differs from the committed value, THEN exactly one request is issued for the
   new value.

8. **Special characters cannot inject a parameter.** GIVEN the milestone title
   `Q3 & Q4=x`, WHEN it is committed, THEN GitLab receives one `milestone`
   parameter whose decoded value is exactly `Q3 & Q4=x`, and the query carries
   no additional or altered parameter.

9. **No match is an empty list, not an error.** GIVEN a milestone title no
   issue carries, WHEN it is committed, THEN the list renders its empty state,
   the badge reads 0, and no error banner is shown.

10. **Merge requests are untouched.** GIVEN the Merge requests view, WHEN it is
    displayed, THEN no milestone control renders, and the request to
    `/api/v1/gitlab/user/mrs` is byte-identical to today's.

11. **Switching selection clears the milestone.** GIVEN a committed milestone
    on the Issues view, WHEN the user selects a different sidebar preset, THEN
    both the milestone draft and its committed value become `""` and the
    refetch carries no `milestone`. WHEN the user instead selects a saved
    query, THEN both become that preset's stored `milestone` (which is `""` for
    one saved before this feature), and the query sent for it uses that
    preset's stored `preset` as its scope. WHEN the user switches to the Merge
    requests view, THEN the milestone clears on that same path — the control is
    not merely unrendered while a live committed value persists behind it — so
    switching back to Issues shows an empty input over an unnarrowed list.

12. **Saving a query captures the trimmed milestone.** GIVEN a milestone
    committed from the draft `"  Next  "` and an otherwise empty custom query
    and project filter, WHEN the user saves the current query, THEN saving is
    offered — `canSaveCurrent` is true **only because it gained the milestone
    term**, since the other two inputs are empty — the persisted `SavedPreset`
    carries `milestone: "Next"` (the committed value, never the raw draft) and
    `preset` set to the effective preset in force at save time, and
    re-selecting the saved query reproduces exactly the result set that was on
    screen at save time.

13. **Legacy saved queries survive.** GIVEN a `gitlab_saved_presets` setting
    written before this feature, containing objects with neither a `milestone`
    nor a `preset` key, WHEN the page loads, THEN every one of those presets is
    listed and selectable, each with `milestone: ""` and `preset: ""`, and none
    is filtered out. WHEN one of them is selected, THEN the scope sent is
    whatever the sidebar preset happens to be, exactly as today — the empty
    stored `preset` restores nothing and changes nothing.

14. **Milestone is visible on the row, in a specified position.** GIVEN an
    issue in milestone `Next` that also carries labels, WHEN it renders in the
    list, THEN the row shows a `Next` chip carrying the milestone test id, and
    that chip appears **after** the `project_path#iid` and "by author ... ago"
    metadata and **before** the first label chip in DOM order; GIVEN an issue
    with no milestone, THEN the row shows no milestone chip and the label chips
    are unmoved.

15. **The mock demonstrates the filter.** GIVEN seeded issues where only some
    carry `milestone: "Next"`, WHEN the milestone `Next` is committed against
    the mock provider, THEN only the matching issues are returned, in
    `ProjectPath` then `IID` ascending order, and repeating the request returns
    the same sequence.

16. **The mock pages the filtered set.** GIVEN 30 seeded issues of which 26
    carry `milestone: "Next"`, WHEN the milestone `Next` is requested with
    `per_page=25`, THEN page 1 returns the first 25 in `ProjectPath` then `IID`
    order with `total_count` 26, page 2 returns the remaining 1 with
    `total_count` 26, and page 3 returns an empty list with `total_count` 26
    and no error.

17. **Degenerate paging arguments do not panic the mock.** GIVEN a Go caller
    invoking `MockClient.ListIssuesPaged` directly with `page` 0 or negative,
    WHEN the call is made, THEN it behaves as `page` 1; and with `perPage` 0 or
    negative it returns no issues. In both cases `TotalCount` is the full
    filtered count and the echoed `Page`/`PerPage` are the normalised values
    used.

18. **Commit normalises the visible input.** GIVEN the milestone input
    containing the draft `"  Next  "`, WHEN the user presses Enter or blurs the
    field, THEN the committed value is `Next` and the input itself also reads
    `Next`; and WHEN the draft is only whitespace, THEN both the committed value
    and the input become empty. GIVEN the user is mid-edit and has typed a
    leading space without committing, THEN that space is preserved in the draft.

19. **A malformed custom query is rejected at the request boundary.** GIVEN
    `%zz`, WHEN `Next` is committed, THEN HTTP 400 is returned, no upstream call
    and the response reports an invalid `custom_query`.

20. **A malformed filter cannot take the milestone down with it.** GIVEN
    `filter=%zz` and an empty custom query, WHEN the milestone `Next` is
    committed, THEN the upstream query carries `milestone=Next` alongside
    `state=opened` and `scope=all`, and the unparseable filter is ignored
    exactly as it is today.

21. **An empty milestone key in a custom query still wins.** GIVEN a custom
    query of `state=closed&milestone=`, WHEN the milestone `Next` is committed,
    THEN the custom query is used unchanged, `Next` is not appended, and GitLab
    receives a single `milestone` parameter whose value is empty.

22. **GitLab's predefined values pass through untranslated.** GIVEN the
    milestone `None`, WHEN it is committed, THEN GitLab receives
    `milestone=None` and kandev applies no special handling; likewise for `Any`.
    GIVEN `Upcoming`, THEN it is forwarded as an ordinary title on `milestone`,
    never on `milestone_id`, and matches only a milestone literally so named.

23. **A legacy preset with a non-string milestone survives.** GIVEN a
    `gitlab_saved_presets` entry whose `milestone` is a number, `null`, or an
    object, WHEN the page loads, THEN `isSavedPreset` accepts the preset, its
    milestone reads as `""`, and it is not filtered out of the list.

24. **The mock honours custom-query precedence.** GIVEN seeded issues in
    milestones `Next` and `Old`, WHEN `MockClient` is called with a custom query
    of `milestone=Old` and the milestone parameter `Next`, THEN only the `Old`
    issues are returned; and WHEN it is called with an empty custom query and
    the parameter `Next`, THEN only the `Next` issues are returned.

25. **The suggested save name follows the precedence ladder.** GIVEN no custom
    query, no project filter, and a committed milestone `Next`, WHEN the save
    dialog opens, THEN the suggested name is `Next`. GIVEN a project filter
    `group/app` in addition, THEN it is `In group/app`. GIVEN a committed custom
    query in addition, THEN it is that custom query. GIVEN none of the three,
    THEN it is `Saved query`.

26. **A milestone chip does not displace a label chip.** GIVEN an issue in
    milestone `Next` carrying six labels, WHEN it renders, THEN the row shows
    the milestone chip, then four label chips, then the `+2` overflow
    indicator — the milestone does not consume one of the four label slots.

27. **The two trims agree on the normative set.** GIVEN the twenty code points
    listed under "Client function, and where trimming happens", WHEN each is
    passed alone through the Go helper and through the TypeScript helper, THEN
    the two agree on every one: the first sixteen yield `""` and the last four
    (U+200B, U+180E, U+2060, U+00B7) are returned unchanged. GIVEN the string
    U+0085 + `Next` + U+FEFF, THEN both yield `Next`. GIVEN `"  Q1 Planning  "`,
    THEN both yield `Q1 Planning` with the interior space intact. This SHALL be
    asserted by a test on each side, and the Go test SHALL additionally assert
    that the helper is not `strings.TrimSpace` by showing U+FEFF alone trims to
    `""`, which `strings.TrimSpace` does not do.

28. **A saved query restores its own scope.** GIVEN the sidebar preset
    **Assigned** is selected and a milestone `Next` is committed, WHEN the user
    saves that query, then selects the sidebar preset **Created**, then selects
    the saved query, THEN the request carries `scope=assigned_to_me` — the
    scope stored with the saved query, not the **Created** scope that was
    selected a moment earlier — and the sidebar selection remains on the saved
    query rather than moving to **Assigned**. GIVEN instead a saved query whose
    stored `preset` is a value no longer present in the issue preset list, THEN
    the request carries an empty `filter`, the list renders normally, and no
    error is surfaced.

29. **Changing the milestone does not strand the project dropdown.** GIVEN the
    Issues view showing results from three projects, WHEN a milestone is
    committed whose issues exist in only one of them, THEN once the new results
    have arrived the dropdown offers that one project and no other. GIVEN the
    intermediate render, where `resetKey` has already changed but the previous
    result set is still on screen because the fetch is in flight, THEN the
    accumulator is **not** refilled with those stale projects. GIVEN instead that
    the user pages forward without changing any filter, THEN projects seen on
    earlier pages remain in the dropdown — the in-flight empty list SHALL NOT
    clear an accumulation whose key did not change. GIVEN a project filter is
    active, THEN its value remains offered regardless, as today.

30. **Deleting the selected saved query clears the milestone too.** GIVEN a
    saved query is selected and its stored `milestone` is `Next`, WHEN the user
    deletes that saved query, THEN the milestone draft and its committed value
    both become `""` in the same update that clears `customQuery` and
    `projectFilter`, and the refetch carries no `milestone`. GIVEN instead that
    the deleted saved query is **not** the selected one, THEN no filter state
    changes and the deletion triggers no refetch.

31. **A repeated milestone key in a custom query resolves to the first.** GIVEN
    the mock client and the custom query `milestone=Old&milestone=Next`, WHEN
    the list is fetched, THEN the mock filters on `Old` — the first value — and
    not on `Next`, and not on both. GIVEN the live path and the same custom
    query, THEN the string is forwarded upstream unchanged, both pairs included,
    and GitLab's own resolution applies.

## Saving a query: when it is offered, and what it is called

**`canSaveCurrent` gains a milestone term.** It is today
`committedQuery.trim().length > 0 || projectFilter.length > 0` and SHALL become
`committedQuery.trim().length > 0 || projectFilter.length > 0 || milestone.length > 0`,
where `milestone` is the **committed** value. A milestone-only query is
therefore saveable, which it is not today. The `.length > 0` form is used for
the milestone rather than a second `.trim()` because the committed value is
already trimmed by definition (see Client function); re-trimming here would
imply a second normalisation boundary where the spec states there is one.

A saved query's `preset` is captured whether or not `canSaveCurrent` was made
true by the milestone — the effective preset is always persisted, including for
a save triggered by a custom query or a project filter alone (see Data model).

**`suggestedLabel`** seeds the save-query dialog's name field. It currently
reads the committed custom query, else `In <project>`, else `Saved query`. With
milestone added the precedence SHALL be, in order:

1. the trimmed committed custom query, when non-empty;
2. `In <projectFilter>`, when a project filter is set;
3. the milestone title, when non-empty;
4. `Saved query`.

The stored `preset` SHALL NOT appear in the suggested name at any rung. It is
restored state, not something the user typed, and adding it would change the
name of queries that have nothing to do with this feature.

Like the existing values, this string is persisted as the query's name and is
therefore i18n-exempt: it must not depend on the creating locale. **Here
`// i18n-exempt: <reason>` genuinely is the correct marker** — this is a plain
expression in a `.tsx` file, invisible to the jsx-only
`i18next/no-literal-string` rule and gated instead by
`scripts/check-nonjsx-copy.mjs`. `gitlab-page-client.tsx:218` already carries
exactly such a marker on today's `suggestedLabel`, and it SHALL be kept.
Contrast the toolbar placeholder under Presentation, which is a JSX attribute
and needs an eslint disable instead; the two markers are different mechanisms
and are not interchangeable.

## Out of scope

Each exclusion below is a contract: these are decided as *not built*, not left
open.

- **`milestone_id`, and its `Upcoming` / `Started` values.** GitLab documents
  `milestone` and `milestone_id` as mutually exclusive, so supporting both means
  a mutual-exclusion rule in the UI and a second control. The issue asks for
  title search. A follow-up wanting `Upcoming`/`Started` would need: a new
  `milestone_id` parameter, a rule rejecting requests that set both, and a UI
  affordance that makes the exclusivity visible.
- **Milestone filtering for merge requests.** GitLab's `/merge_requests`
  accepts the same attribute and the change would be symmetrical, but the issue
  is scoped to issues and MR search has its own preset set including
  `review_requested`. Not built; `GET /user/mrs` is asserted unchanged above.
- **A milestone dropdown or autocomplete.** Requires enumerating milestones
  across every visible project; GitLab has no cross-project milestone endpoint.
  A follow-up would need to decide which projects to enumerate, how to cache
  the result, and what to show before it loads.
- **Milestone on GitLab issue watches.** `IssueWatch` has a structured `labels`
  field and could take a milestone the same way, but a watch is a poller, not a
  search, and its dialog already documents `state=opened&milestone=Next` in the
  custom-query placeholder — the capability is reachable there today. A
  follow-up would need a DB column, a store migration, `applyIssueWatchPatch`
  handling, and a fold in `fetchIssues` next to the existing label fold.
- **GitHub, Linear, Jira or Azure DevOps parity.** No other integration gains a
  milestone filter, and none of their request shapes change.
- **Teaching `MockClient` any filter key other than `milestone`.** `state`,
  `scope`, `labels` and `assignee_username` continue to be ignored by the mock.
- **Sorting or `order_by` controls.** Upstream order is preserved for the live
  path; the only ordering this feature specifies is the mock's.
- **Persisting the milestone as page state across reloads.** Matches the
  existing behaviour of the project filter and custom query.
- **Multiple milestones at once.** GitLab's `milestone` attribute takes one
  title. One input, one value.

## Files touched

Enumerated because the milestone's transport is an interface signature change,
and that is exactly the kind of change whose blast radius a planner under-counts.

**Backend, `apps/backend/internal/gitlab/` only:**

| File | Change |
|---|---|
| `client.go` | `Client` interface: the new `milestone` parameter on `ListIssues` and `ListIssuesPaged`, and their doc comments |
| `client_helpers.go` | `buildIssueSearchQuery` takes the milestone; the custom-query fold; `rawIssue` gains its milestone sub-struct and `convertRawIssue` reads its `title` |
| `models.go` | `Issue.Milestone` |
| `pat_client.go` | both methods forward the new parameter |
| `mock_client.go` | both methods: effective-milestone resolution, filtering, the named sort, and paging |
| `noop_client.go` | both signatures; the `ErrNoClient` bodies are unchanged |
| `controller.go` | `httpSearchUserIssues` reads and trims `milestone`, then passes it |
| `service_search.go` | `SearchUserIssues` and `SearchUserIssuesPaged` gain the parameter; the stats call passes `""` |
| `service_issue_watches.go` | call site passes `""` — mechanical, not a feature change |
| `service_mentions.go` | call site passes `""` |

`rawIssue` and `convertRawIssue` live in `client_helpers.go`, **not** in
`models.go`, which holds only the `Issue` type. `glab_client.go` needs no change:
`GLabClient` embeds `*PATClient` and inherits both methods.
`apps/backend/internal/github/**` needs no change and SHALL NOT be touched.

**Frontend:**

| File | Change |
|---|---|
| `apps/web/lib/types/gitlab.ts` | `Issue.milestone`, mirroring the Go model |
| `apps/web/lib/api/domains/gitlab-api.ts` | `searchUserIssues` accepts an optional `milestone`, setting the param only when non-empty |
| `apps/web/components/gitlab/my-gitlab/use-gitlab-search.ts` | milestone in the hook options, in `FetchArgs`, in the fetch effect's dependencies, and in `refresh` |
| `apps/web/app/gitlab/gitlab-page-client.tsx` | loses `useGitLabPageState`; wires the toolbar control |
| `apps/web/app/gitlab/use-gitlab-page-state.ts` | **new file** — the extracted hook, `useProjectOptions`, `initialSelection`, `GitLabPageState`, `useSaveQueryDialog`, and the milestone state |
| `apps/web/components/gitlab/my-gitlab/use-saved-presets.ts` | `SavedPreset.milestone`, `SavedPreset.preset`, and the shared `isSavedPreset` tolerance rule covering both |
| `apps/web/components/gitlab/my-gitlab/list-toolbar.tsx` | the milestone input and its five new props (below) |
| `apps/web/components/gitlab/my-gitlab/issue-list.tsx` | the milestone chip and its test id |
| `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/` | the control's visible label and accessible name |

`list-toolbar.tsx` is on this list because the milestone input renders inside it,
and it has to be, because **there is exactly one `ListToolbar` call site**
(`gitlab-page-client.tsx:301`) and it serves the Issues view and the Merge
requests view alike. A second call site is not an option, so the Issues-only
rule is a prop rather than a rendering location. Its five new props are REQUIRED,
not optional, and are specified under Presentation.

`components/integrations/integration-list-toolbar.tsx` is **not** on this list.
`ListToolbar` composes it and passes the milestone input down as part of the
existing `filter` slot; the shared integration toolbar's own props do not change,
so GitHub and every other integration that renders it are untouched.

`use-committed-query.ts` is deliberately absent from this list and SHALL NOT be
edited. The milestone reuses it as-is, one more instance of the draft/committed
pair it already provides; nothing about the hook needs to know a milestone
exists.

## Constraints

- **The extraction is specified, not left to the implementer.**
  `apps/web/app/gitlab/gitlab-page-client.tsx` is 589 lines against a 600-line
  file cap (`max-lines`, warn), and `useGitLabPageState` spans lines 161-264 —
  **104 lines, already over the 100-line `max-lines-per-function` cap** before
  this feature adds anything. An in-file comment at line 156 records that
  `useProjectOptions` was already lifted out for this reason. Adding milestone
  state in place would push both further over, so the implementation SHALL:
  - move `useGitLabPageState`, its co-located helpers `useProjectOptions` and
    `initialSelection`, and the `GitLabPageState` type into a new
    `apps/web/app/gitlab/use-gitlab-page-state.ts`, leaving
    `gitlab-page-client.tsx` well under the file cap; and
  - within that new file, extract the save-query concern into a
    `useSaveQueryDialog` hook, so `useGitLabPageState` itself lands **below**
    100 lines with the milestone state added rather than merely relocating an
    existing warning. What moves, named as the file actually names it today:
    `saveDialogOpen` / `setSaveDialogOpen`, `canSaveCurrent`, `suggestedLabel`,
    and the `onOpenSaveDialog`, `onConfirmSave` and `onDeleteSaved` handlers.
    There is no `saveName` state to move: the dialog's name field is local
    `value` state inside `SavePresetForm` (`save-preset-dialog.tsx`), it is not
    lifted into the page hook today, and this feature SHALL NOT lift it.

  The milestone's draft/committed pair SHALL reuse the existing
  `useCommittedQuery` hook rather than adding a second mechanism, and SHALL use
  it **unmodified**. The hook supplies `draft`, `committed`, `setDraft`,
  `setImmediate` and `commit`; the trimming boundary is met by committing
  through `setImmediate(trimGitLabMilestone(draft))`, as
  specified under "Client function, and where trimming happens". `commit()`
  copies the draft verbatim and SHALL NOT be changed, because the custom-query
  input shares this hook.
  `suggestedLabel` and `canSaveCurrent` move with the dialog hook and gain the
  milestone term defined under "Saving a query: when it is offered, and what it
  is called".
- New copy must land in all five catalogs (`en`, `pt-pt`, `zh-cn`, `zh-hk`,
  `zh-tw`); `check-i18n-keys.mjs` fails on a missing key, a dropped
  placeholder, or a value left identical to English. Use `pnpm run i18n:zh-hant`
  for the Traditional Chinese pair.
- No Unicode em dash (U+2014) in any user-facing copy or locale value.
- Backend and frontend `Issue` types must change together;
  `apps/web/lib/types/gitlab.ts` mirrors `apps/backend/internal/gitlab/models.go`.

## Related

- Issue: https://github.com/kdlbs/kandev/issues/2798
- `docs/specs/gitlab-integration/` — the umbrella GitLab integration spec
- `apps/backend/internal/integrations/AGENTS.md` — integration conventions
