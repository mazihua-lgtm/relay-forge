"""Read-only check: has upstream (QuantumNous/new-api) moved since the last sync?

  check.py [--repo <path>] [--since <upstream-sha>] [--no-fetch] [--report-file <path>]

The last synced upstream commit is the "Upstream-Commit:" trailer on the tip of
the upstream-sync branch. The script fetches upstream's main (the only thing it
changes in the repo is the upstream/main ref: no merge, no checkout, no push),
lists the commits since then, marks the ones that look security related, and
names the files that need a closer read: dependencies, workflows, scripts,
licenses. RELAY-FORGE.md ("同步上游") says what to do with the answer.

Exit status: 0 nothing new, 10 new commits, 2 the check itself failed (a failed
check must never read as "nothing new").

Environment:
  UPSTREAM_REMOTE       remote name (default: upstream)
  UPSTREAM_BASE_REF     branch whose tip carries the trailer (default: upstream-sync)
  UPSTREAM_PROXY        http proxy for git, tried first, then a direct fetch
                        (default: this machine's http://127.0.0.1:7891; empty = direct only)
  UPSTREAM_FETCH_ARGS   extra arguments for git fetch, e.g. --filter=blob:none

Run it as `python3 -I`."""
import argparse
import os
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

REMOTE = os.environ.get("UPSTREAM_REMOTE", "upstream")
BASE_REF = os.environ.get("UPSTREAM_BASE_REF", "upstream-sync")
PROXY = os.environ.get("UPSTREAM_PROXY", "http://127.0.0.1:7891")
FETCH_ARGS = os.environ.get("UPSTREAM_FETCH_ARGS", "").split()

# Words that make a commit worth reading first. The first group is flagged
# wherever it appears; the second only on a "fix" commit, where it names what was
# fixed rather than a feature that happens to touch authentication.
STRONG = re.compile(
    r"secur|vulnerab|\bCVE-|\bXSS\b|\bCSRF\b|\bSSRF\b|inject|overflow|\bleak|expos|"
    r"privilege|escalat|bypass|sanitiz|越权|泄露|泄漏|漏洞|安全|绕过|注入|私有|private",
    re.I,
)
WEAK = re.compile(
    r"auth|token|password|secret|credential|permission|鉴权|认证|权限|密钥|密码", re.I
)
# Files whose change deserves a closer read before anything is merged.
WATCH = re.compile(
    r"(^|/)(go\.(mod|sum)|package\.json|bun\.lockb?|pnpm-lock\.yaml|package-lock\.json|"
    r"yarn\.lock|Dockerfile[^/]*|docker-compose[^/]*\.ya?ml|LICENSE|NOTICE|"
    r"THIRD-PARTY-LICENSES\.md|AGENTS\.md|CLAUDE\.md|[^/]+\.sh|[Mm]akefile)$|^\.github/"
)


class CheckFailed(Exception):
    pass


def git(repo, *args, check=True):
    result = subprocess.run(
        ["git", "-C", str(repo), *args],
        capture_output=True, text=True, encoding="utf-8", errors="replace",
    )
    if check and result.returncode != 0:
        raise CheckFailed(f"git {' '.join(args[:2])} failed: {result.stderr.strip()[-300:]}")
    return result


def recorded_base(repo):
    result = git(repo, "log", "-1", "--format=%B", BASE_REF, check=False)
    if result.returncode != 0:
        raise CheckFailed(
            f"branch {BASE_REF} not found here. It records which upstream commit was last "
            f"synced; see RELAY-FORGE.md, 同步上游."
        )
    match = re.search(r"^Upstream-Commit:\s*([0-9a-f]{40})\s*$", result.stdout, re.M)
    if not match:
        raise CheckFailed(f"the tip of {BASE_REF} has no 'Upstream-Commit:' line")
    return match.group(1)


def fetch(repo):
    if git(repo, "remote", "get-url", REMOTE, check=False).returncode != 0:
        raise CheckFailed(
            f"remote '{REMOTE}' does not exist. Add it: git remote add {REMOTE} "
            f"https://github.com/QuantumNous/new-api.git"
        )
    attempts = []
    if PROXY:
        # An explicit proxy is ignored when NO_PROXY is set, so that attempt drops it.
        env = {k: v for k, v in os.environ.items() if k not in ("NO_PROXY", "no_proxy")}
        attempts.append((["-c", "http.version=HTTP/1.1", "-c", f"http.proxy={PROXY}"], env))
    attempts.append(([], dict(os.environ)))
    last = ""
    for options, env in attempts:
        env.update(GIT_HTTP_LOW_SPEED_LIMIT="1000", GIT_HTTP_LOW_SPEED_TIME="60", GIT_TERMINAL_PROMPT="0")
        try:
            result = subprocess.run(
                ["git", "-C", str(repo), *options, "fetch", "--no-tags", *FETCH_ARGS, REMOTE, "main"],
                capture_output=True, text=True, encoding="utf-8", errors="replace",
                env=env, timeout=300,
            )
        except subprocess.TimeoutExpired:
            last = "timed out"
            continue
        if result.returncode == 0:
            return
        last = (result.stderr.strip().splitlines() or ["no output"])[-1]
    raise CheckFailed(f"could not fetch {REMOTE}/main: {last}")


def flag(subject, body):
    """The words that make a commit worth reading first, or None."""
    text = subject + "\n" + body
    found = STRONG.search(text)
    if found:
        return found.group(0).lower()
    if re.match(r"fix", subject, re.I):
        found = WEAK.search(text)
        if found:
            return found.group(0).lower()
    return None


def commits_since(repo, base, tip):
    raw = git(repo, "log", "--reverse", "--format=%H%x1f%an%x1f%as%x1f%s%x1f%b%x1e", f"{base}..{tip}").stdout
    commits = []
    for record in raw.split("\x1e"):
        record = record.strip("\n")
        if not record:
            continue
        sha, author, date, subject, body = record.split("\x1f", 4)
        commits.append({"sha": sha, "author": author, "date": date, "subject": subject, "flag": flag(subject, body)})
    return commits


def changed_files(repo, base, tip):
    raw = git(repo, "diff", "--name-status", "--no-renames", "-z", base, tip).stdout.split("\0")
    return [(raw[i], raw[i + 1]) for i in range(0, len(raw) - 1, 2)]


def report(base, tip, commits, files, overridden):
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    lines = ["# 上游检查（relay-forge）", ""]
    start = f"`{base[:7]}`" + ("（手动指定的起点，测试用）" if overridden else "（上次同步到这里）")
    if not commits:
        lines += [f"上游没有新提交：起点 {start}，上游现在也在 `{tip[:7]}`。检查时间 {now}。"]
        return "\n".join(lines) + "\n"
    lines += [
        f"上游有 **{len(commits)} 个新提交**，涉及 {len(files)} 个文件。起点 {start}，上游现在在 `{tip[:7]}`。检查时间 {now}。",
        "",
        "## 新提交",
        "",
    ]
    for c in commits:
        mark = f" ⚠ 可能与安全有关（命中：{c['flag']}）" if c["flag"] else ""
        lines.append(f"- `{c['sha'][:7]}` {c['date']} {c['author']}：{c['subject']}{mark}")
    watched = [(s, p) for s, p in files if WATCH.search(p)]
    if watched:
        names = {"A": "新增", "M": "修改", "D": "删除", "T": "类型变化"}
        lines += ["", "## 同步前要重点审的文件（依赖、工作流、脚本、许可证）", ""]
        lines += [f"- {names.get(s, s)} `{p}`" for s, p in watched]
    lines += [
        "",
        "## 下一步",
        "",
        "先读 diff 再决定拿不拿：带 ⚠ 的提交请先读具体改动。要同步就按 `RELAY-FORGE.md` 的「同步上游」一节做，或者直接叫我同步。这个检查只读，不会自己合并。",
    ]
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description="Has upstream moved since the last sync? Read-only.")
    parser.add_argument("--repo", default=str(Path(__file__).resolve().parents[2]))
    parser.add_argument("--since", help="use this upstream commit as the start instead of the recorded one (testing)")
    parser.add_argument("--no-fetch", action="store_true", help="do not fetch; use the upstream/main already here")
    parser.add_argument("--report-file", help="also write the report to this file")
    args = parser.parse_args()
    repo = Path(args.repo)

    try:
        # Fetch first, so that --since can name a commit this clone has not seen yet.
        if not args.no_fetch:
            fetch(repo)
        if args.since:
            base = git(repo, "rev-parse", "--verify", f"{args.since}^{{commit}}", check=False).stdout.strip()
            if not base:
                raise CheckFailed(f"--since {args.since}: no such commit here")
        else:
            base = recorded_base(repo)
        tip = git(repo, "rev-parse", "--verify", f"{REMOTE}/main^{{commit}}", check=False).stdout.strip()
        if not tip:
            raise CheckFailed(f"{REMOTE}/main does not exist here; fetch it first")
        if git(repo, "cat-file", "-e", f"{base}^{{commit}}", check=False).returncode != 0:
            raise CheckFailed(f"the recorded upstream commit {base[:7]} is not in this repository's objects")
        if git(repo, "merge-base", "--is-ancestor", base, tip, check=False).returncode != 0:
            raise CheckFailed(
                f"the last synced commit {base[:7]} is not in the history of {REMOTE}/main. "
                f"Upstream may have rewritten its history: do not sync until that is understood."
            )
        commits = commits_since(repo, base, tip)
        files = changed_files(repo, base, tip) if commits else []
    except CheckFailed as error:
        print(f"上游检查失败（这不等于没有新提交）：{error}", file=sys.stderr)
        return 2

    text = report(base, tip, commits, files, overridden=bool(args.since))
    sys.stdout.write(text)
    if args.report_file:
        Path(args.report_file).write_text(text, encoding="utf-8")
    return 10 if commits else 0


if __name__ == "__main__":
    sys.exit(main())
