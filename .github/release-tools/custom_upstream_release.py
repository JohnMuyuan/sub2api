"""Resolve only the latest published upstream release to its tagged commit."""
import json
import os
from pathlib import Path
import subprocess
import sys

UPSTREAM = "ranxi2001/sub2api"
RELEASE_REF = "refs/custom-upstream/latest-release"


def run(*args):
    return subprocess.run(args, check=True, capture_output=True, text=True,
                          encoding="utf-8").stdout.strip()


def fetch_release_commit(release, remote="upstream"):
    if release.get("draft") is not False or release.get("prerelease") is not False:
        raise ValueError("Only published, non-prerelease releases can be synced")
    tag = release.get("tag_name")
    if not isinstance(tag, str) or not tag:
        raise ValueError("Release has no tag")
    run("git", "check-ref-format", f"refs/tags/{tag}")
    # Fetch the tag itself: target_commitish can point at an unreleased branch.
    run("git", "fetch", "--no-tags", remote,
        f"+refs/tags/{tag}:{RELEASE_REF}")
    return run("git", "rev-parse", "--verify", f"{RELEASE_REF}^{{commit}}")


def main():
    release = json.loads(run("gh", "api", f"repos/{UPSTREAM}/releases/latest"))
    sha = fetch_release_commit(release)
    message = f"Official upstream release: {release['tag_name']} ({sha})"
    print(message, file=sys.stderr)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with Path(os.environ["GITHUB_STEP_SUMMARY"]).open("a", encoding="utf-8") as summary:
            summary.write(message + "\n")
    print(sha)


if __name__ == "__main__":
    main()
