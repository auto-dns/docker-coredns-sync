## Summary

<!-- What does this change and why? -->

## Linked issues

<!--
Use a GitHub closing keyword so the issue auto-closes when the release branch
merges to main, and gets the `awaiting-release` label when this PR merges into a
release branch. e.g. "Closes #123".
-->
Closes #

## Checklist

- [ ] Targets the active release branch (`vMAJOR.MINOR.PATCH`), not `main`
- [ ] `go test -race ./...` passes and `make lint` is clean
- [ ] Updated `CHANGELOG.md` under `## [Unreleased]`
