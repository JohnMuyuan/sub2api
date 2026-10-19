"""Exercise a real upstream merge of the frontend test-list insertion point."""
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
CUSTOM_TESTS = [
    "src/components/account/__tests__/QuotaDimensionRow.siteTimezone.spec.ts",
    "src/components/common/__tests__/DateRangePicker.siteTimezone.spec.ts",
    "src/utils/__tests__/siteTimezone.spec.ts",
    "src/utils/__tests__/formatDateLocalInput.spec.ts",
    "src/components/settings/SystemTimezoneSettings.spec.ts",
    "src/stores/__tests__/auth.spec.ts",
]

class CustomSyncTest(unittest.TestCase):
    def test_critical_test_paths_exist_and_are_unique(self):
        manifest = (ROOT / "Makefile").read_text(encoding="utf-8")
        manifest += (ROOT / "Makefile.custom").read_text(encoding="utf-8")
        paths = re.findall(r"src/[^\s\\]+\.spec\.ts", manifest)
        self.assertEqual(len(paths), len(set(paths)))
        for path in CUSTOM_TESTS:
            self.assertTrue((ROOT / "frontend" / path).is_file(), path)

    def test_upstream_can_prepend_tests_while_retaining_custom_regressions(self):
        fork = (ROOT / "Makefile").read_text(encoding="utf-8")
        extension = ROOT / "Makefile.custom"
        custom = extension.read_text(encoding="utf-8") if extension.exists() else ""
        base = "\n".join(line for line in fork.splitlines()
                         if not any(name in line for name in CUSTOM_TESTS)
                         and "include Makefile.custom" not in line) + "\n"
        with tempfile.TemporaryDirectory() as temp:
            work = Path(temp)
            def git(*args, check=True):
                return subprocess.run(["git", *args], cwd=work, text=True, encoding="utf-8",
                                      capture_output=True, check=check)
            git("init", "-q")
            git("config", "user.name", "Sync test")
            git("config", "user.email", "sync@example.invalid")
            def commit(message):
                git("add", ".")
                git("commit", "-qm", message)
            (work / "Makefile").write_text(base, encoding="utf-8")
            commit("upstream base")
            base_sha = git("rev-parse", "HEAD").stdout.strip()
            git("checkout", "-qb", "personal")
            (work / "Makefile").write_text(fork, encoding="utf-8")
            if custom:
                (work / "Makefile.custom").write_text(custom, encoding="utf-8")
            commit("personal regression tests")
            git("checkout", "-qb", "upstream", base_sha)
            future = "src/future-upstream.spec.ts"
            marker = "FRONTEND_CRITICAL_VITEST := " + chr(92) + "\n"
            self.assertIn(marker, base)
            (work / "Makefile").write_text(base.replace(marker, marker + "\t" + future + " " + chr(92) + "\n", 1), encoding="utf-8")
            commit("upstream adds a new test at the usual insertion point")
            git("checkout", "personal")
            merged = git("merge", "--no-edit", "upstream", check=False)
            self.assertEqual(0, merged.returncode, merged.stdout + merged.stderr)
            combined = (work / "Makefile").read_text(encoding="utf-8")
            if (work / "Makefile.custom").exists():
                combined += (work / "Makefile.custom").read_text(encoding="utf-8")
            for test in [future, *CUSTOM_TESTS]:
                self.assertIn(test, combined)
            if shutil.which("make"):
                expanded = subprocess.run(["make", "-n", "test-frontend-critical"], cwd=work,
                                          text=True, capture_output=True, check=True).stdout
                for test in [future, *CUSTOM_TESTS]:
                    self.assertIn(test, expanded)

if __name__ == "__main__":
    unittest.main()
