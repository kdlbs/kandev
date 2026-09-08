# Harness Validation

Use this reference for the shared checks after changing skills, agents,
references, or always-on instruction files.

```bash
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- <changed-files>
wc -l <changed-harness-files>
```

Before committing, also run the targeted pre-commit hook:

```bash
pre-commit run harness-lint --files <changed-files>
```

Keep every `AGENTS.md` and `CLAUDE.md` file at 300 lines or fewer. Move detail
to the nearest scoped file or a referenced document when a parent reaches the
limit.
