"""Tests for check.py, with no network: a throwaway repository stands in for upstream
and another for relay-forge, so every way the check can go wrong is exercised,
above all the ones where it could say "nothing new" when it does not know.

  python3 -I scripts/upstream-sync/test_check.py"""
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

CHECK = Path(__file__).resolve().parent / "check.py"
ENV = {
    **os.environ,
    "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
    "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
    "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_SYSTEM": os.devnull,
}


def git(repo, *args):
    return subprocess.run(["git", "-C", str(repo), *args], env=ENV, check=True, capture_output=True, text=True).stdout.strip()


def commit(repo, files, message):
    for name, text in files.items():
        path = Path(repo, name)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "-m", message)
    return git(repo, "rev-parse", "HEAD")


class CheckTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        root = Path(self.tmp.name)
        self.upstream, self.work = root / "upstream", root / "work"
        for repo in (self.upstream, self.work):
            repo.mkdir()
            git(repo, "init", "-q", "-b", "main")
        self.base = commit(self.upstream, {"a.txt": "1"}, "feat: first")
        git(self.work, "remote", "add", "upstream", str(self.upstream))
        git(self.work, "fetch", "-q", "--no-tags", "upstream", "main")
        self.record(self.base)

    def tearDown(self):
        self.tmp.cleanup()

    def record(self, sha):
        """What relay-forge's upstream-sync branch says was last synced."""
        git(self.work, "checkout", "-q", "-B", "upstream-sync")
        git(self.work, "commit", "-q", "--allow-empty", "-m", f"vendor: test\n\nUpstream-Commit: {sha}")

    def check(self, *args, **env):
        result = subprocess.run(
            [sys.executable, "-I", str(CHECK), "--repo", str(self.work), *args],
            env={**ENV, "UPSTREAM_PROXY": "", **env}, capture_output=True, text=True, encoding="utf-8",
        )
        return result.returncode, result.stdout, result.stderr

    def test_nothing_new(self):
        code, out, _ = self.check()
        self.assertEqual(code, 0)
        self.assertIn("没有新提交", out)

    def test_new_commits_are_listed_oldest_first_and_security_ones_flagged(self):
        commit(self.upstream, {"b.txt": "1"}, "feat: add colors")
        commit(self.upstream, {"c.txt": "1"}, "fix: leak of private data in the export")
        code, out, _ = self.check()
        self.assertEqual(code, 10)
        lines = [line for line in out.splitlines() if line.startswith("- `")]
        self.assertEqual(len(lines), 2)
        self.assertIn("feat: add colors", lines[0])
        self.assertNotIn("⚠", lines[0])
        self.assertIn("fix: leak of private data", lines[1])
        self.assertIn("⚠", lines[1])

    def test_weak_words_flag_a_fix_but_not_a_feature(self):
        commit(self.upstream, {"b.txt": "1"}, "feat: new token page")
        commit(self.upstream, {"c.txt": "1"}, "fix: token refresh race")
        _, out, _ = self.check()
        lines = [line for line in out.splitlines() if line.startswith("- `")]
        self.assertNotIn("⚠", lines[0])
        self.assertIn("⚠", lines[1])

    def test_a_word_in_the_body_counts(self):
        commit(self.upstream, {"b.txt": "1"}, "fix: export\n\nThis closes an XSS hole.")
        _, out, _ = self.check()
        self.assertIn("⚠", out)

    def test_files_that_need_a_closer_read_are_named(self):
        commit(self.upstream, {"go.mod": "x", "web/package.json": "x", ".github/workflows/ci.yml": "x", "docs/readme.md": "x"}, "chore: bump")
        code, out, _ = self.check()
        self.assertEqual(code, 10)
        self.assertIn("同步前要重点审的文件", out)
        for name in ("go.mod", "web/package.json", ".github/workflows/ci.yml"):
            self.assertIn(f"`{name}`", out)
        self.assertNotIn("docs/readme.md", out)

    def test_no_closer_read_section_when_nothing_sensitive_changed(self):
        commit(self.upstream, {"b.txt": "1"}, "feat: add colors")
        _, out, _ = self.check()
        self.assertNotIn("同步前要重点审的文件", out)

    def test_since_overrides_the_recorded_start_and_says_so(self):
        first = commit(self.upstream, {"b.txt": "1"}, "feat: second")
        commit(self.upstream, {"c.txt": "1"}, "feat: third")
        code, out, _ = self.check("--since", first)
        self.assertEqual(code, 10)
        self.assertIn("feat: third", out)
        self.assertNotIn("feat: second", out)
        self.assertIn("手动指定", out)

    def test_report_file_equals_stdout(self):
        commit(self.upstream, {"b.txt": "1"}, "feat: add colors")
        target = Path(self.tmp.name) / "report.md"
        _, out, _ = self.check("--report-file", str(target))
        self.assertEqual(target.read_text(encoding="utf-8"), out)

    def test_rewritten_upstream_history_is_a_failure_not_nothing_new(self):
        git(self.upstream, "checkout", "-q", "--orphan", "other")
        git(self.upstream, "rm", "-rfq", ".")
        commit(self.upstream, {"z.txt": "1"}, "feat: unrelated history")
        git(self.upstream, "branch", "-M", "main")
        code, _, err = self.check()
        self.assertEqual(code, 2)
        self.assertIn("rewritten", err)

    def test_missing_remote_is_a_failure(self):
        git(self.work, "remote", "remove", "upstream")
        code, _, err = self.check()
        self.assertEqual(code, 2)
        self.assertIn("does not exist", err)

    def test_missing_base_branch_is_a_failure(self):
        git(self.work, "checkout", "-q", "--detach")
        git(self.work, "branch", "-D", "upstream-sync")
        code, _, err = self.check()
        self.assertEqual(code, 2)
        self.assertIn("upstream-sync", err)

    def test_missing_trailer_is_a_failure(self):
        git(self.work, "commit", "-q", "--allow-empty", "-m", "no trailer here")
        code, _, err = self.check()
        self.assertEqual(code, 2)
        self.assertIn("Upstream-Commit", err)

    def test_a_failed_fetch_is_a_failure_not_nothing_new(self):
        commit(self.upstream, {"b.txt": "1"}, "feat: add colors")
        git(self.work, "remote", "set-url", "upstream", str(Path(self.tmp.name) / "nowhere"))
        code, out, err = self.check()
        self.assertEqual(code, 2)
        self.assertEqual(out, "")
        self.assertIn("could not fetch", err)

    def test_no_fetch_uses_what_is_already_here(self):
        commit(self.upstream, {"b.txt": "1"}, "feat: add colors")
        code, _, _ = self.check("--no-fetch")
        self.assertEqual(code, 0)


if __name__ == "__main__":
    unittest.main()
