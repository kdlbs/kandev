## Office specification authority

Use `docs/specs/office/` for Office requirements and system designs.
If code and a specification disagree, identify the conflict; do not silently follow either.

Find the current documents with:

```sh
python3 scripts/list-docs.py specs --system office --format paths
```

Add `--kind legacy` when the change involves a legacy specification.
Read the amendment in `docs/specs/office/system-design/runtime-01.md` before relying on its provider recovery rules.
`docs/specs/office/requirements/routing.md` is deprecated. Use the current tier-selection and dynamic-routing specifications.
