"""Whole-tree proof for the upstream sync (RELAY-FORGE.md, "同步上游"):
<vendor commit>'s tree == rewrite(<upstream commit>'s tree), file by file.

  verify_tree.py <repo> <vendor> <upstream>

Non-Go files must be byte-identical; Go files and go.mod must equal upstream's
after the module-path rewrite (and gofmt, for Go files). Exits 1 on any
difference. Run it as `python3 -I`."""
import subprocess, sys

repo, vendor, upstream = sys.argv[1:4]
OLD, NEW = b"github.com/QuantumNous/new-api", b"github.com/mazihua-lgtm/relay-forge"

def git(*args, inp=None):
    return subprocess.run(["git", "-C", repo, *args], capture_output=True, check=True, input=inp).stdout

def tree(commit):
    out = git("ls-tree", "-r", "-z", commit).decode().split("\0")
    res = {}
    for line in out:
        if not line:
            continue
        meta, path = line.split("\t", 1)
        mode, kind, sha = meta.split()
        res[path] = (mode, sha)
    return res

v, u = tree(vendor), tree(upstream)
only_v, only_u = sorted(set(v) - set(u)), sorted(set(u) - set(v))
print(f"files: vendor={len(v)} upstream={len(u)}  only in vendor={len(only_v)}  only in upstream={len(only_u)}")
bad, checked_go = [], 0
for path, (mode, sha) in u.items():
    if path not in v:
        continue
    vmode, vsha = v[path]
    if vmode != mode:
        bad.append((path, f"mode {vmode} != {mode}")); continue
    base = path.rsplit("/", 1)[-1]
    if path.endswith(".go") or base == "go.mod":
        data = git("cat-file", "blob", sha).replace(OLD, NEW)
        if path.endswith(".go"):
            data = subprocess.run(["gofmt"], input=data, capture_output=True, check=True).stdout
            checked_go += 1
        if data != git("cat-file", "blob", vsha):
            bad.append((path, "content differs after rewrite+gofmt"))
    elif vsha != sha:
        bad.append((path, "blob differs"))
print(f"Go files compared after rewrite+gofmt: {checked_go}")
print(f"mismatches: {len(bad)}")
for p, why in bad[:10]:
    print("  ", p, "-", why)
for p in only_v[:5]: print("   only in vendor:", p)
for p in only_u[:5]: print("   only in upstream:", p)
sys.exit(1 if bad or only_v or only_u else 0)
