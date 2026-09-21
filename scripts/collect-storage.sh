# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0

# Read-only collector for Scout's storage.json.
# Run this on a machine that can already see Ceph, ZFS, NFS, or Kubernetes.
# It writes a local file. It does not upload anything.

set -eu
out=${1:-storage.json}
tmp=$(mktemp)

python3 - "$out" <<'PY'
import json, os, shutil, subprocess, sys

def run(cmd):
    if shutil.which(cmd[0]) is None:
        return None
    try:
        return subprocess.check_output(cmd, text=True, stderr=subprocess.DEVNULL)
    except subprocess.CalledProcessError:
        return None

doc = {}
ceph = run(["ceph", "df", "-f", "json"])
if ceph:
    raw = json.loads(ceph)
    pools = []
    for pool in raw.get("pools", []):
        stats = pool.get("stats", {})
        pools.append({
            "name": pool.get("name", ""),
            "usedBytes": int(stats.get("bytes_used", 0)),
            "maxBytes": int(stats.get("max_avail", 0) + stats.get("bytes_used", 0)),
        })
    if pools:
        doc["ceph"] = {"pools": pools}

zfs = run(["zfs", "list", "-Hp", "-o", "name,used,avail"])
if zfs:
    datasets = []
    for line in zfs.splitlines():
        parts = line.split("\t")
        if len(parts) != 3:
            continue
        datasets.append({"name": parts[0], "usedBytes": int(parts[1]), "availBytes": int(parts[2])})
    if datasets:
        doc["zfs"] = {"datasets": datasets}

server = os.environ.get("NFS_SERVER", "")
show = run(["showmount", "-e", server]) if server else run(["showmount", "-e"])
if show:
    exports = []
    for line in show.splitlines()[1:]:
        path = line.split()[0] if line.split() else ""
        if path:
            exports.append({"server": server, "path": path})
    if exports:
        doc["nfs"] = {"exports": exports}

if shutil.which("kubectl"):
    def names(kind):
        raw = run(["kubectl", "get", kind, "-A", "-o", "json"])
        if not raw:
            return []
        body = json.loads(raw)
        return body.get("items", [])
    classes = [item.get("metadata", {}).get("name", "") for item in names("storageclass")]
    pvs = names("pv")
    pvcs = names("pvc")
    doc["kubernetes"] = {
        "storageClasses": [c for c in classes if c],
        "persistentVolumes": len(pvs),
        "persistentVolumeClaims": len(pvcs),
    }

path = sys.argv[1]
with open(path, "w", encoding="utf-8") as fh:
    json.dump(doc, fh, indent=2)
    fh.write("\n")
os.chmod(path, 0o600)
print(f"wrote {path}")
PY
rm -f "$tmp"
