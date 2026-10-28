#!/usr/bin/env python3
"""Lane 3: re-run Search A's 6,024 inaccessible rows through the engine with a
working provider. Fresh ledger; same IdentityRow construction as the B runner."""
from __future__ import annotations

import json
import os
import sys
import threading
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(ROOT, "scripts"))

import company_careers as cc  # noqa: E402

OUTDIR = os.path.join(ROOT, "artifacts", "search A", "a-recovery-run")
QUEUE = os.path.join(ROOT, "artifacts", "search A", "remedy-queues.json")
UNLIMITED = 1_000_000_000
cc.RUN_DEADLINE_SECONDS = 10_000_000_000.0
cc.GLOBAL_PAGE_ATTEMPTS = 1_000_000_000

import urllib.request  # noqa: E402


class NullAdapter(cc.SearchAdapter):
    """DDG placeholder — blocked from this IP; keeps the adapter list shape."""
    name = "ddg"
    env_key = "DDG_ENABLED"
    def available(self): return False
    def search(self, query, count=5, http_impl=None):
        raise cc.ProviderError("rate", "ddg blocked", 403)


def build_all():
    cc.ADAPTER_CLASSES[:] = [cc.ParallelAdapter, NullAdapter]
    cc.make_outdirs(OUTDIR)
    conn = cc.open_ledger(os.path.join(OUTDIR, "ledger.sqlite3"))
    with conn:
        for k, v in {"global_cap": UNLIMITED, "firstpass_cap": UNLIMITED,
                     "followup_cap": UNLIMITED, "audit_cap": UNLIMITED,
                     "version": "a-recovery-1"}.items():
            conn.execute("INSERT OR REPLACE INTO meta(k,v) VALUES(?,?)", (k, str(v)))
    _, rs, budget, fetcher, sc, runtime = cc.build_engine(OUTDIR)
    sc.search_rate = cc.RateLimiter(6.0, 8)
    sc.inflight = cc.SharedInflight(8)
    sc.provider_rate["parallel"] = cc.RateLimiter(6.0, 6)
    q = json.load(open(QUEUE))
    with_ats = {r["source_id"]: r for r in q["with_ats"]}
    rows = []
    for r in q["without_ats"]:
        ir = cc.IdentityRow(id=r["source_id"], name=r["name"] or "", name_variants="",
                            urls=0, hn_urls=0, distinct_titles=0, top_locations="",
                            sample_url=(r["urls"][0] if r["urls"] else ""),
                            sources="", review_status="", jev={}, site_evidence=[],
                            historical={})
        rows.append((ir, r))
    return conn, rs, fetcher, sc, runtime, rows, with_ats


def cmd_run():
    conn, rs, fetcher, sc, runtime, rows, with_ats = build_all()
    done = rs.done_ids()
    queue = [(ir, meta) for ir, meta in rows if ir.id not in done]
    total = len(queue)
    print(f"run: {total} inaccessible rows to re-process (6 workers)", flush=True)
    t0 = time.monotonic()
    lock = threading.Lock()
    counter = {"n": 0}
    statuses = {}

    def work(item):
        ir, meta = item
        try:
            rr = cc.process_row(ir.id, ir, sc, fetcher, rs, OUTDIR, runtime)
        except Exception as e:  # noqa: BLE001
            rr = cc.finish_untouched(ir.id, ir, rs, OUTDIR, "interrupted", repr(e)[:200])
        rs.set_done(ir.id, rr, OUTDIR)
        return rr

    from concurrent.futures import ThreadPoolExecutor, as_completed
    with ThreadPoolExecutor(max_workers=6) as pool:
        futures = {pool.submit(work, item): item for item in queue}
        for fut in as_completed(futures):
            rr = fut.result()
            with lock:
                counter["n"] += 1
                statuses[rr.identity_status] = statuses.get(rr.identity_status, 0) + 1
                n = counter["n"]
            if n % 100 == 0 or n == total:
                rate = n / max(time.monotonic() - t0, 1e-6)
                print(f"run: {n}/{total} ({rate:.2f} rows/s) "
                      f"statuses={sorted(statuses.items(), key=lambda kv: -kv[1])[:6]}", flush=True)
                json.dump({"done": n, "total": total, "statuses": statuses, "ts": time.time()},
                          open(os.path.join(OUTDIR, "progress.json"), "w"))
                recent = conn.execute("SELECT provider, outcome FROM search_reservations "
                                      "ORDER BY res_id DESC LIMIT 12").fetchall()
                auth = sum(1 for r in recent if r["provider"] == "parallel" and r["outcome"] == "auth")
                if auth >= 8:
                    print("PARALLEL-DEAD: stopping; resume after top-up.", flush=True)
                    break
    print("run: complete", json.dumps(statuses), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(cmd_run())
