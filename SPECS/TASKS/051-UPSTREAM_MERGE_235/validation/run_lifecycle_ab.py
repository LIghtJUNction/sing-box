#!/usr/bin/env python3
"""Measure fixed sing-tun lifecycle paths; copies module sources, never edits cache."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import statistics
import subprocess
import tempfile

VERSIONS = {
    "old": "v0.9.7-0.20261002083955-3f8acd9da65b",
    "new": "v0.9.7-0.20261028024126-5b0a1a7a394e",
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go", help="Go 1.25+ executable")
    parser.add_argument("--runs", type=int, default=3)
    parser.add_argument("--work-dir", type=Path, help="new directory for source copies and evidence")
    args = parser.parse_args()
    if args.runs < 1:
        parser.error("--runs must be positive")
    work = args.work_dir
    if work is None:
        work = Path(tempfile.mkdtemp(prefix="sing-tun-lifecycle-ab-"))
    else:
        work.mkdir(parents=True, exist_ok=False)
    work = work.resolve()
    env = dict(os.environ, GOTOOLCHAIN="local", GOWORK="off", GOMAXPROCS="1", GOFLAGS="")
    probe = Path(__file__).with_name("lifecycle_ab_test.go.txt").read_bytes()
    summary = {
        "go": subprocess.check_output([args.go, "version"], env=env, text=True).strip(),
        "GOMAXPROCS": 1, "build_tags": "default", "runs": args.runs,
        "scope": "controlled flow-table HeapAlloc after GC and live NAT map entries; not RSS",
        "results": {},
    }
    module_hashes = []
    for label, version in VERSIONS.items():
        downloaded = subprocess.check_output(
            [args.go, "mod", "download", "-json", "github.com/sagernet/sing-tun@" + version],
            cwd=work, env=env, text=True,
        )
        source = Path(json.loads(downloaded)["Dir"])
        destination = work / label
        shutil.copytree(source, destination)
        destination.chmod(0o755)
        (destination / "lifecycle_ab_test.go").write_bytes(probe)
        flags = ", flags" if label == "new" else ""
        (destination / "lifecycle_ab_adapter_test.go").write_text(
            'package tun\nimport ("net/netip"; "github.com/sagernet/sing-tun/gtcpip/header")\n'
            '// The old public API cannot observe TCP flags.\n'
            'func abLookup(n *TCPNat, s, d netip.AddrPort, flags header.TCPFlags) uint16 '
            '{ return n.Lookup(s, d' + flags + ') }\n'
            'func abLookupBack(n *TCPNat, p uint16, flags header.TCPFlags) *TCPSession '
            '{ return n.LookupBack(p' + flags + ') }\n'
        )
        module_hashes.append(hashlib.sha256((destination / "go.mod").read_bytes()).hexdigest())
        log_path = work / (label + ".log")
        command = [args.go, "test", "-p=1", "-mod=readonly", "-run", "^TestLifecycleAB$",
                   "-v", "-count=" + str(args.runs), "-timeout=60s", "."]
        with log_path.open("w") as log:
            subprocess.run(command, cwd=destination, env=env, stdout=log,
                           stderr=subprocess.STDOUT, check=True)
        rows = [json.loads(line.split("LIFECYCLE_AB ", 1)[1])
                for line in log_path.read_text().splitlines() if "LIFECYCLE_AB " in line]
        if len(rows) != args.runs * 10:
            raise RuntimeError(f"{label}: expected {args.runs * 10} observations, got {len(rows)}")
        for row in rows:
            expected = row["count"] if label == "old" or row["scenario"] in ("nat_active", "nat_half_fin") else 0
            observed = row.get("retained_entries", row.get("retained_sessions"))
            if observed != expected or row.get("retained_addresses", expected) != expected:
                raise RuntimeError(f"unexpected retained state for {label}: {row}")
        heaps = {
            str(count): statistics.median(row["heap_delta_bytes"] for row in rows
                                          if row["scenario"] == "accepted_ack" and row["count"] == count)
            for count in (1000, 10000)
        }
        summary["results"][label] = {"version": version, "observations": rows, "median_heap_delta_bytes": heaps}
    if len(set(module_hashes)) != 1:
        raise RuntimeError("new and old go.mod differ; the dependency comparison requires review")
    summary["go_mod_sha256"] = module_hashes[0]
    result_path = work / "results.json"
    result_path.write_text(json.dumps(summary, indent=2) + "\n")
    print(result_path)


if __name__ == "__main__":
    main()
