# Move the smolquery image to the org namespace

## Problem

smolquery moved from `chasers/smolquery` to the `smolquery` org. `release.yml` still pushes `ghcr.io/chasers/smolquery`, a package that a user account owns. The org repo's `GITHUB_TOKEN` cannot write it: `denied: permission_denied: The requested installation does not exist` (smolquery run 38333094929). No commit after `df8fe23` has an image.

## Steps

- [x] smolquery PR: #474 was closed as a duplicate of #464, which also moves the repository links. #464 squash-merged as `a8a561a` after a merge-from-main fix for a release-notes conflict. The original task text: `GHCR_IMAGE: ghcr.io/smolquery/smolquery`, and the README and deployment docs.
- [x] bench-matrix: look for `ghcr.io/smolquery/smolquery` first, and fall back to `ghcr.io/chasers/smolquery` for commits up to `df8fe23`.
- [x] After the first push: make the new package public. GitHub created it private; the org first had to allow public packages (Settings → Packages → Package creation).
- [x] Release run 38404792723 for `a8a561a` pushed `ghcr.io/smolquery/smolquery:sha-a8a561a…` and dispatched bench-matrix run 38405413514 (`BENCH_MATRIX_TOKEN` works).
- [ ] Out of scope, for the user: `smolquery-deploy` (`supabase/o11y-team-tools`) pins `ghcr.io/chasers/smolquery` in `overlay/sandbox/kustomization.yaml`.
