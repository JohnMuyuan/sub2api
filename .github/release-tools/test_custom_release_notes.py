import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import custom_release_notes as notes

ROOT = Path(__file__).resolve().parents[2]


def release(tag, body="change notes"):
    return {"tag_name": tag, "html_url": f"https://github.com/{notes.UPSTREAM}/releases/tag/{tag}", "body": body}


class CustomReleaseNotesTest(unittest.TestCase):
    def setUp(self):
        previous = Path.cwd()
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.addCleanup(os.chdir, previous)
        os.chdir(temp.name)
        notes.run("git", "init", "-q")
        notes.run("git", "config", "user.name", "Release test")
        notes.run("git", "config", "user.email", "test@example.invalid")
        Path("README.md").write_text("upstream README\n", encoding="utf-8")
        Path(".gitattributes").write_text((ROOT / ".gitattributes").read_text(encoding="utf-8"), encoding="utf-8")
        self.base = self.commit("base")
        self.old = self.commit("upstream 1")
        self.new = self.commit("upstream 2")
        self.source = self.commit("personal changes")
        self.future = self.commit("unmerged upstream 3")
        for tag, sha in [("v1", self.old), ("v2", self.new), ("v3", self.future)]:
            notes.run("git", "update-ref", notes.UPSTREAM_REFS + tag, sha)
        notes.run("git", "checkout", "--detach", self.source)
        self.releases = [release("v3", "FUTURE NOTES"), release("v2", "NEW NOTES\n\n```text\nquotes '$()' stay data\n```"), release("v1", "OLD NOTES")]

    def commit(self, value):
        Path("changes.txt").write_text(value, encoding="utf-8")
        notes.run("git", "add", ".")
        notes.run("git", "commit", "-qm", value)
        return notes.run("git", "rev-parse", "HEAD").stdout.strip()

    def test_includes_only_new_merged_release(self):
        body = notes.render("2026.10.2", self.source, self.releases, self.old)
        self.assertIn("NEW NOTES", body)
        self.assertIn("quotes '$()' stay data", body)
        self.assertNotIn("OLD NOTES", body)
        self.assertNotIn("FUTURE NOTES", body)
        self.assertIn("上游基线：[v2]", body)

    def test_multiple_upstream_releases_are_chronological(self):
        body = notes.render("2026.10.2", self.source, self.releases, self.base)
        self.assertLess(body.index("OLD NOTES"), body.index("NEW NOTES"))
        self.assertNotIn("FUTURE NOTES", body)

    def test_personal_only_release_still_shows_current_upstream_notes(self):
        body = notes.render("2026.10.2", self.source, self.releases, self.source)
        self.assertIn("本次没有新增上游正式版本", body)
        self.assertIn("NEW NOTES", body)
        self.assertNotIn("OLD NOTES", body)

    def test_first_release_uses_latest_merged_release(self):
        body = notes.render("2026.10.2", self.source, self.releases, None)
        self.assertIn("NEW NOTES", body)
        self.assertNotIn("OLD NOTES", body)

    def test_unmerged_release_is_not_misrepresented(self):
        body = notes.render("2026.10.2", self.base, self.releases, None)
        self.assertIn("未匹配到已合入的上游正式发布", body)
        self.assertNotIn("NEW NOTES", body)

    def test_rerun_and_unmerged_personal_release_do_not_change_baseline(self):
        for tag, sha in [("v2026.10.3", self.future), ("v2026.10.2", self.source), ("v2026.10.1", self.old)]:
            notes.run("git", "tag", tag, sha)
        own = [release("v2026.10.3"), release("v2026.10.2"), release("v2026.10.1")]
        self.assertEqual("refs/tags/v2026.10.1", notes.previous_release(own, "v2026.10.2", self.source))

    def test_keep_upgrade_notes_but_omit_upstream_install_footer(self):
        body = "Changes\n\n## 升级注意\nKeep this.\n\n---\n\n## 📥 Installation\nUPSTREAM INSTALL COMMAND\n"
        self.assertEqual("Changes\n\n## 升级注意\nKeep this.", notes.changelog(body))
        self.assertEqual("", notes.changelog(None))
        self.assertEqual("normal notes", notes.changelog("normal notes"))

    def test_pagination_filters_drafts_and_prereleases(self):
        import json
        from types import SimpleNamespace
        pages = [[dict(release("old"), draft=False, prerelease=False, published_at="2026-01-01")], [dict(release("draft"), draft=True, prerelease=False, published_at="2026-02-01"), dict(release("preview"), draft=False, prerelease=True, published_at="2026-02-01"), dict(release("new"), draft=False, prerelease=False, published_at="2026-02-01")]]
        with patch.object(notes, "run", return_value=SimpleNamespace(stdout=json.dumps(pages))) as call:
            self.assertEqual(["new", "old"], [item["tag_name"] for item in notes.releases("owner/repo")])
        self.assertIn("--paginate", call.call_args.args)

    def test_auto_merge_preserves_personal_readme_and_other_upstream_changes(self):
        notes.run("git", "checkout", "-B", "upstream", self.old)
        Path("README.md").write_text("new upstream README\n", encoding="utf-8")
        Path("new-feature.txt").write_text("upstream feature", encoding="utf-8")
        notes.run("git", "add", ".")
        notes.run("git", "commit", "-qm", "upstream README and feature")
        notes.run("git", "checkout", "-B", "custom", self.source)
        Path("README.md").write_text("江木源的自用版本\n", encoding="utf-8")
        notes.run("git", "add", ".")
        notes.run("git", "commit", "-qm", "personal README")
        notes.run("git", "-c", "merge.keep-custom-readme.driver=true", "merge", "--no-edit", "upstream")
        self.assertEqual("江木源的自用版本\n", Path("README.md").read_text(encoding="utf-8"))
        self.assertEqual("upstream feature", Path("new-feature.txt").read_text(encoding="utf-8"))
        workflow = (ROOT / ".github/workflows/custom-upstream-sync.yml").read_text(encoding="utf-8")
        self.assertIn("git -c merge.keep-custom-readme.driver=true merge --no-edit upstream/production", workflow)


if __name__ == "__main__":
    unittest.main()
