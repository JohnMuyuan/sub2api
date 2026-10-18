# Personal automatic updates

## What happens automatically

- `Custom upstream sync` checks ranxi2001/sub2api's `production` branch hourly
  (GitHub scheduled jobs may be delayed). It merges into `sync/upstream`,
  preserving the custom branch's history and features.
- The proposed commit runs focused custom-feature Go regression tests, the
  persistent-update shell test, and the complete Docker frontend/backend build.
  These are not the project's full integration tests.
- On success, a PR records the change and a non-forced fast-forward updates
  `custom` to the exact validated commit. Conflicts, failed checks, or concurrent
  personal commits stop the update; they require manual resolution.
- The workflow explicitly dispatches `Custom build`: GitHub's own token does
  not trigger ordinary push workflows. Human pushes/PR merges into `custom`
  also trigger publication automatically.
- One Linux amd64 Docker build produces the `custom`, numeric version, and
  `sha-<commit>` image tags, plus the matching executable tarball and SHA-256
  checksums in a GitHub Release. The matching managed-login Python/Node runtime
  is built and validated with the existing upstream tooling, and is also
  downloaded from this fork for this version. The release stays draft until assets and images
  are uploaded. The existing general upstream Release workflow is unchanged.
- Versions use `year.month.workflow-run-number` (for example `2026.10.12`),
  independent of upstream version numbers. Release notes identify the source
  commit; even a small personal feature gets a detectable new version.

## First migration from an older Docker image

1. Back up the database and preserve deployment environment variables/keys.
2. After the first Custom build publication succeeds, use 1Panel to force-pull
   `ghcr.io/johnmuyuan/sub2api:custom` and recreate the application container.
3. Keep the existing persistent mount at `/app/data`, and use a Docker restart
   policy such as `unless-stopped` or `always`.
4. Subsequent built-in update checks query `JohnMuyuan/sub2api` releases. Click
   update, then the existing restart button when prompted. The database and
   configuration remain on their original volumes. Checks may be cached for
   20 minutes; a forced check refreshes them immediately.

Custom images opt into a writable executable at `/app/data/runtime/sub2api`.
The updater verifies the published checksum and atomically replaces that file;
the built-in rollback can restore its previous executable. Restarting or
recreating the same image with the same data mount retains the updated version.
Explicitly switching to a different image seeds that image's executable and
clears the old executable backup, so a pinned image can be used for recovery.
Do not share one runtime data directory between multiple running replicas.

Built-in update changes the application executable (including embedded frontend),
not the Docker base OS, PostgreSQL client, or other image resources. Pull and
recreate the image when those dependencies change. No Docker socket is needed.
The updater itself does not run automatically: you choose when to install a
published version. New versions continue to contain custom changes as long as
merge conflicts are resolved while retaining those changes.
