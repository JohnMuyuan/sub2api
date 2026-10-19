"""Release sync must stop at a published tag even when its branch advances."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from custom_upstream_release import fetch_release_commit


class UpstreamReleaseTest(unittest.TestCase):
    def test_only_tagged_commit_is_selected_for_both_tag_types(self):
        with tempfile.TemporaryDirectory() as temp:
            previous = Path.cwd()
            try:
                os.chdir(temp)
                def git(*args):
                    return subprocess.run(["git", *args], check=True, capture_output=True,
                                          text=True, encoding="utf-8").stdout.strip()
                git("init", "-q")
                git("config", "user.name", "Release test")
                git("config", "user.email", "test@example.invalid")
                git("commit", "--allow-empty", "-qm", "published")
                published = git("rev-parse", "HEAD")
                git("tag", "v1.0.0")
                git("tag", "-a", "v1.0.1", "-m", "annotated release")
                git("commit", "--allow-empty", "-qm", "unreleased branch change")
                unreleased = git("rev-parse", "HEAD")
                self.assertNotEqual(published, unreleased)
                for tag in ["v1.0.0", "v1.0.1"]:
                    with self.subTest(tag=tag):
                        release = dict(tag_name=tag, draft=False, prerelease=False,
                                       target_commitish=unreleased)
                        sha = fetch_release_commit(release, remote=".")
                        self.assertEqual(published, sha)
            finally:
                os.chdir(previous)

    def test_drafts_prereleases_and_missing_tags_are_rejected(self):
        for release in [dict(tag_name="v2", draft=True, prerelease=False),
                        dict(tag_name="v2-rc1", draft=False, prerelease=True),
                        dict(draft=False, prerelease=False)]:
            with self.subTest(release=release), self.assertRaises(ValueError):
                fetch_release_commit(release)

    def test_workflow_uses_release_resolver_instead_of_branch(self):
        workflow = (Path(__file__).resolve().parents[1] / "workflows" /
                    "custom-upstream-sync.yml").read_text(encoding="utf-8")
        self.assertIn("release_sha=$(python .github/release-tools/custom_upstream_release.py)", workflow)
        self.assertIn('merge --no-edit "$release_sha"', workflow)
        self.assertNotIn("upstream/production", workflow)


if __name__ == "__main__":
    unittest.main()
