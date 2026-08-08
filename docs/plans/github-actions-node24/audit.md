# Action runtime audit procedure

Goal: verify that every `uses:` pin in `.github/workflows/` runs on the
Node 24 action runtime (or is composite / docker://), so no Node-20
deprecation warnings are emitted.

## Step 1 — collect pins

```bash
grep -rhoE "uses: [^ ]+@[0-9a-f]{40}" .github/workflows/*.yml | sort | uniq -c | sort -rn
```

## Step 2 — check each pinned SHA's runtime

For each `owner/repo@<40-hex>` pin, fetch the action manifest at that exact
commit and read `runs.using`:

```bash
curl -sf "https://raw.githubusercontent.com/<owner>/<repo>/<sha>/action.yml" \
  | grep -A1 "^runs:"
# action.yml may be action.yaml in some repos; try both.
```

Acceptable values: `node24` (good), `composite` (no runtime of its own —
inspect inner `uses:` steps, e.g. `actions/upload-pages-artifact` v5 embeds
`actions/upload-artifact@v7.0.0` which is node24). Reject `node20`.

## Step 3 — dereference annotated tags

Some pins are the SHA of an annotated tag object rather than a commit
(e.g. `pnpm/action-setup@a8198c4… # v5`). Dereference before judging:

```bash
gh api repos/<owner>/<repo>/git/tags/<sha> --jq '.object.sha'
```

## Step 4 — resolve target SHAs (when upgrading)

For each target tag, resolve the commit SHA and confirm node24:

```bash
sha=$(gh api "repos/<owner>/<repo>/git/refs/tags/<tag>" --jq '.object.sha')
# if .object.type == "tag", dereference via git/tags/<sha> → .object.sha
curl -sf "https://raw.githubusercontent.com/<owner>/<repo>/$sha/action.yml" | grep -A1 "^runs:"
```

## Step 5 — final assertion

After the upgrade, rerun steps 1–2 and assert:

- every `runs.using:` is `node24` or `composite`;
- every ref is still a 40-char SHA (also covered by `lint-action-pinning.py`);
- no output line contains `node20`.

Last verified clean: 2026-08-07 (all pins node24/composite — see plan.md
target table).
