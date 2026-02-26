# Contributing to Kandev

Contributions are welcome! This document covers the basics.

## Important

You must understand the code you submit. You're welcome to use AI tools to help write code, but every PR will be reviewed by a human maintainer and the feature you're contributing should be manually tested. If you can't explain what your code does and why, it's not ready to submit.

## How to Contribute

1. **Fork and branch.** Create a feature branch from `main`.
2. **Keep PRs focused.** One logical change per PR. Small PRs get reviewed faster.sdfs
3. **Test your changes.** Run `make typecheck test lint` before submitting. Manually verify that your feature works end-to-end, add screenshots or recordings to the PR if it has a UI component.

## Bug Reports

Search [existing issues](https://github.com/cflynn7/kandev/issues) first. If your bug isn't already reported, open one with:

- Steps to reproduce
- Expected vs actual behavior
- Environment details (OS, browser, agent type)

## Feature Ideas

Open an issue describing the feature and the problem it solves. Keep it concise.

## Code Quality

New code must pass the existing linters and tests:

```bash
make test       # Backend + web tests
make lint       # Backend + web linters
make typecheck  # TypeScript type checking
```

## License

By contributing, you agree that your contributions will be licensed under the [AGPL-3.0](LICENSE) license.
