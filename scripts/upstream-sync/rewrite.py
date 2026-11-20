"""Module-path rewrite for the upstream-sync branch (byte exact, no newline games).

Two jobs, both used by the upstream sync described in RELAY-FORGE.md ("同步上游"):
  rewrite.py all    <worktree>                          rewrite every tracked Go file and go.mod
  rewrite.py delta  <repo> <worktree> <base> <upstream> copy upstream's changes (base..upstream) in, rewritten
It prints the Go files it wrote, one per line, on stdout (the caller runs gofmt -w on them).

Run it as `python3 -I`, from outside the tree it works on."""
import os, subprocess, sys

OLD, NEW = b"github.com/QuantumNous/new-api", b"github.com/mazihua-lgtm/relay-forge"

def rewritable(path):
    return path.endswith(".go") or os.path.basename(path) == "go.mod"

def git(repo, *args):
    return subprocess.run(["git", "-C", repo, *args], capture_output=True, check=True).stdout

mode = sys.argv[1]
if mode == "all":
    wt = sys.argv[2]
    for path in git(wt, "ls-files", "-z").decode().split("\0"):
        if not path or not rewritable(path):
            continue
        full = os.path.join(wt, path)
        data = open(full, "rb").read()
        new = data.replace(OLD, NEW)
        if new != data:
            open(full, "wb").write(new)
        if path.endswith(".go"):
            print(path)
elif mode == "delta":
    repo, wt, base, up = sys.argv[2:6]
    raw = git(repo, "diff", "--name-status", "--no-renames", "-z", base, up).decode().split("\0")
    entries = [(raw[i], raw[i + 1]) for i in range(0, len(raw) - 1, 2)]
    for status, path in entries:
        full = os.path.join(wt, path)
        if status == "D":
            os.remove(full)
            continue
        assert status in ("A", "M"), (status, path)
        data = git(repo, "show", f"{up}:{path}")
        if rewritable(path):
            data = data.replace(OLD, NEW)
        os.makedirs(os.path.dirname(full), exist_ok=True)
        # keep the executable bit of the upstream blob
        mode_line = git(repo, "ls-tree", up, "--", path).decode().split()[0]
        open(full, "wb").write(data)
        os.chmod(full, 0o755 if mode_line == "100755" else 0o644)
        if path.endswith(".go"):
            print(path)
        print(f"{status} {path}", file=sys.stderr)
