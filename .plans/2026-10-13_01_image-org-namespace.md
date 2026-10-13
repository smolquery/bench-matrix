# Move the smolquery image to the org namespace

## Problem

smolquery moved from `chasers/smolquery` to the `smolquery` org. `release.yml` still pushes `ghcr.io/chasers/smolquery`, a package that a user account owns. The org repo's `GITHUB_TOKEN` cannot write it: `denied: permission_denied: The requested installation does not exist` (smolquery run 38333094929). No commit after `df8fe23` has an image.

## Steps

- [x] smolquery PR (smolquery/smolquery#474): `GHCR_IMAGE: ghcr.io/smolquery/smolquery`, and the README and deployment docs.
- [x] bench-matrix: look for `ghcr.io/smolquery/smolquery` first, and fall back to `ghcr.io/chasers/smolquery` for commits up to `df8fe23`.
- [ ] After the first push: make the new package public if GitHub created it private.
- [ ] Out of scope, for the user: `smolquery-deploy` (`supabase/o11y-team-tools`) pins `ghcr.io/chasers/smolquery` in `overlay/sandbox/kustomization.yaml`.
