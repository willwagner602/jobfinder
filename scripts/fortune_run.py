#!/usr/bin/env python3
"""Fortune 1000 missing-names careers run: same engine as search_b_full_run,
over the 370 Fortune companies not already covered."""
from __future__ import annotations

import json
import os
import sys
import threading
import time

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(ROOT, "scripts"))

import company_careers as cc  # noqa: E402

BASE = os.path.join(ROOT, "artifacts", "search B", "company-list-sanity")
MISSING = os.path.join(BASE, "search-b-full", "fortune-missing.json")
OUTDIR = os.path.join(BASE, "fortune-run")
UNLIMITED = 1_000_000_000

cc.RUN_DEADLINE_SECONDS = 10_000_000_000.0

import hashlib  # noqa: E402
import html as _html  # noqa: E402
import re as _re  # noqa: E402
import urllib.parse as _up  # noqa: E402
import urllib.request as _ur  # noqa: E402


class DuckDuckGoAdapter(cc.SearchAdapter):
    name = "ddg"
    env_key = "DDG_ENABLED"
    _UA = "Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0"

    def available(self):
        return True

    def search(self, query, count=5, http_impl=None):
        last = None
        for mirror in ("https://html.duckduckgo.com/html/", "https://lite.duckduckgo.com/lite/"):
            try:
                return self._go(mirror, query, count, http_impl)
            except cc.ProviderError as e:
                last = e
                if e.kind == "rate":
                    continue
                raise
        raise last or cc.ProviderError("server", "all ddg mirrors failed")

    def _go(self, base, query, count, http_impl):
        url = base + "?q=" + _up.quote_plus(query)
        if http_impl is not None:
            status, raw = http_impl("GET", url, {"User-Agent": self._UA}, None, timeout=15.0)
        else:
            req = _ur.Request(url, headers={"User-Agent": self._UA})
            with _ur.urlopen(req, timeout=15.0) as resp:
                status, raw = resp.status, resp.read()
        if status in (403, 429) or b"captcha" in raw[:4000].lower() or b"anomaly" in raw[:4000].lower():
            raise cc.ProviderError("rate", "ddg block", status)
        if status != 200:
            raise cc.ProviderError("server" if status >= 500 else "client", f"ddg {status}", status)
        page = raw.decode("utf-8", "replace")
        out = []
        for m in _re.finditer(r'<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>', page):
            u = self._clean(m.group(1)); t = _re.sub(r"<[^>]+>", "", m.group(2))
            if u:
                out.append(cc.SearchResult(self.name, u, _html.unescape(t).strip(), "", len(out) + 1, {}))
            if len(out) >= count:
                break
        return out

    @staticmethod
    def _clean(href):
        if href.startswith("//"):
            href = "https:" + href
        if "uddg=" in href:
            q = _up.parse_qs(_up.urlparse(href).query)
            if q.get("uddg"):
                return q["uddg"][0]
        return href if href.startswith("http") else ""


def load_rows():
    miss = json.load(open(MISSING))
    return [{"source_id": f"F{m['rank']:04d}", "observed_name": m["name"],
             "name_variants": "", "location_context": "",
             "sources": "fortune1000-2025", "site_evidence_classes": "",
             "site_evidence_roots": ""} for m in miss]


def to_identity_row(d):
    return cc.IdentityRow(id=d["source_id"], name=d["observed_name"], name_variants="",
                          urls=0, hn_urls=0, distinct_titles=0, top_locations="",
                          sample_url="", sources=d["sources"], review_status="",
                          jev={}, site_evidence=[], historical={})


def build_all():
    cc.ADAPTER_CLASSES[:] = [cc.ParallelAdapter, DuckDuckGoAdapter]
    cc.GLOBAL_PAGE_ATTEMPTS = 1_000_000_000
    cc.make_outdirs(OUTDIR)
    conn = cc.open_ledger(os.path.join(OUTDIR, "ledger.sqlite3"))
    with conn:
        for k, v in {"global_cap": UNLIMITED, "firstpass_cap": UNLIMITED,
                     "followup_cap": UNLIMITED, "audit_cap": UNLIMITED,
                     "version": "fortune-run-1"}.items():
            conn.execute("INSERT OR REPLACE INTO meta(k,v) VALUES(?,?)", (k, str(v)))
    conn2, rs, budget, fetcher, sc, runtime = cc.build_engine(OUTDIR)
    sc.search_rate = cc.RateLimiter(6.0, 8)
    sc.inflight = cc.SharedInflight(8)
    sc.provider_rate["parallel"] = cc.RateLimiter(6.0, 6)
    sc.provider_rate["ddg"] = cc.RateLimiter(0.5, 1)
    rows = load_rows()
    identities = {r["source_id"]: to_identity_row(r) for r in rows}
    return conn, rs, fetcher, sc, runtime, identities


def cmd_run():
    conn, rs, fetcher, sc, runtime, identities = build_all()
    done = rs.done_ids()
    queue = [identities[k] for k in identities if k not in done]
    total = len(queue)
    print(f"run: {total} Fortune rows to process (6 workers)", flush=True)
    t0 = time.monotonic()
    lock = threading.Lock()
    counter = {"n": 0}
    statuses = {}

    def work(ir):
        try:
            rr = cc.process_row(ir.id, ir, sc, fetcher, rs, OUTDIR, runtime)
        except Exception as e:  # noqa: BLE001
            rr = cc.finish_untouched(ir.id, ir, rs, OUTDIR, "interrupted", repr(e)[:200])
        rs.set_done(ir.id, rr, OUTDIR)
        return rr

    from concurrent.futures import ThreadPoolExecutor, as_completed
    with ThreadPoolExecutor(max_workers=6) as pool:
        futures = {pool.submit(work, ir): ir for ir in queue}
        for fut in as_completed(futures):
            rr = fut.result()
            with lock:
                counter["n"] += 1
                statuses[rr.identity_status] = statuses.get(rr.identity_status, 0) + 1
                n = counter["n"]
            if n % 25 == 0 or n == total:
                print(f"run: {n}/{total} statuses={sorted(statuses.items(), key=lambda kv: -kv[1])}",
                      flush=True)
                json.dump({"done": n, "total": total, "statuses": statuses, "ts": time.time()},
                          open(os.path.join(OUTDIR, "progress.json"), "w"))
    json.dump({"done": counter["n"], "total": total, "statuses": statuses, "complete": True,
               "ts": time.time()}, open(os.path.join(OUTDIR, "progress.json"), "w"))
    print("run: complete", json.dumps(statuses), flush=True)
    return 0


def main():
    return cmd_run()


if __name__ == "__main__":
    raise SystemExit(main())
