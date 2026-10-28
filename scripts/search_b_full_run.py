#!/usr/bin/env python3
"""Search B full-list runner: drives the company_careers engine over frozen-worklist.json.

Operator directives in effect (2026-10-25):
  - budget: unlimited (meta caps set to 1e9; no pilot cohort)
  - checkpoint gates: already unlimited (checkpoint_remaining_quota returns 10M)
  - the 800 wave-001 rows are pre-seeded as done in the engine ledger so the
    run covers exactly the remaining ~12,636 rows and one merge covers all.

Usage:
  python3 scripts/search_b_full_run.py prepare      # build ledger, seed done rows
  python3 scripts/search_b_full_run.py run [--limit N]   # resume-until-done loop
  python3 scripts/search_b_full_run.py status       # progress summary
"""
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
WAVE = os.path.join(BASE, "careers-wave-001")
WORKLIST = os.path.join(WAVE, "frozen-worklist.json")
OUTDIR = os.path.join(BASE, "search-b-full")
UNLIMITED = 1_000_000_000

# No 3-hour deadline: this run goes until the worklist is done.
cc.RUN_DEADLINE_SECONDS = 10_000_000_000.0

# ---------------------------------------------------------------------------
# Free DuckDuckGo HTML adapter (no key). Only Parallel + DDG have usable
# credentials today; the other four provider keys are quota-dead (brave 402,
# exa credits, tavily plan limit, kagi 401, you payment_required).
import hashlib  # noqa: E402
import html as _html  # noqa: E402
import re as _re  # noqa: E402
import urllib.parse as _up  # noqa: E402
import urllib.request as _ur  # noqa: E402


class DuckDuckGoAdapter(cc.SearchAdapter):
    """HTML-scrape adapter for html.duckduckgo.com / lite.duckduckgo.com."""

    name = "ddg"
    env_key = "DDG_ENABLED"
    _UA = ("Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0")

    def available(self) -> bool:
        return True

    def search(self, query, count=5, http_impl=None):
        last_err: Exception | None = None
        for mirror in ("https://html.duckduckgo.com/html/", "https://lite.duckduckgo.com/lite/"):
            try:
                return self._search_mirror(mirror, query, count, http_impl)
            except cc.ProviderError as e:
                last_err = e
                if e.kind == "rate":
                    continue  # try next mirror on block
                raise
        raise last_err or cc.ProviderError("server", "all ddg mirrors failed")

    def _search_mirror(self, base, query, count, http_impl):
        url = base + "?q=" + _up.quote_plus(query) + f"&s={hashlib.md5(query.encode()).hexdigest()[:8]}"
        if http_impl is not None:
            status, raw = http_impl("GET", url, {"User-Agent": self._UA}, None, timeout=15.0)
        else:
            req = _ur.Request(url, headers={"User-Agent": self._UA})
            with _ur.urlopen(req, timeout=15.0) as resp:
                status, raw = resp.status, resp.read()
        if status in (403, 429) or b"anomaly" in raw[:4000].lower() or b"captcha" in raw[:4000].lower():
            raise cc.ProviderError("rate", "ddg block/captcha", status)
        if status != 200:
            raise cc.ProviderError("server" if status >= 500 else "client",
                                   f"ddg status {status}", status)
        return self._parse(raw.decode("utf-8", "replace"), count)

    def _parse(self, page: str, count: int):
        out: list[cc.SearchResult] = []
        # html.duckduckgo.com: <a rel="nofollow" class="result__a" href="...">Title</a>
        for m in _re.finditer(
                r'<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>', page):
            url = self._clean_url(m.group(1))
            title = _re.sub(r"<[^>]+>", "", m.group(2))
            if not url:
                continue
            out.append(cc.SearchResult(self.name, url, _html.unescape(title).strip(), "", len(out) + 1, {}))
            if len(out) >= count:
                return out
        # lite.duckduckgo.com: <a rel="nofollow" href="...">Title</a> inside table
        if not out:
            for m in _re.finditer(r'<a[^>]+rel="nofollow"[^>]+href="([^"]+)"[^>]*>(.*?)</a>', page):
                url = self._clean_url(m.group(1))
                title = _re.sub(r"<[^>]+>", "", m.group(2))
                if not url:
                    continue
                out.append(cc.SearchResult(self.name, url, _html.unescape(title).strip(), "",
                                           len(out) + 1, {}))
                if len(out) >= count:
                    break
        return out

    @staticmethod
    def _clean_url(href: str) -> str:
        if href.startswith("//"):
            href = "https:" + href
        if "uddg=" in href:
            q = _up.parse_qs(_up.urlparse(href).query)
            if q.get("uddg"):
                return q["uddg"][0]
        if href.startswith("http"):
            return href
        return ""


def load_worklist() -> list[dict]:
    return json.load(open(WORKLIST))


def to_identity_row(d: dict) -> cc.IdentityRow:
    roots = [r.strip() for r in (d.get("site_evidence_roots") or "").split(";") if r.strip()]
    classes = [c.strip() for c in (d.get("site_evidence_classes") or "").split(";") if c.strip()]
    sources = d.get("sources") or ""
    evs: list[cc.EvidenceUrl] = []
    for i, r in enumerate(roots):
        cls = classes[i] if i < len(classes) else (classes[0] if classes else "Unknown")
        evs.append(cc.EvidenceUrl(url=r, url_class=cls, role="lead", kind="site",
                                  distinct_urls=1, samples=[r], sources=sources))
    return cc.IdentityRow(
        id=d["source_id"], name=d.get("observed_name") or "", name_variants=d.get("name_variants") or "",
        urls=0, hn_urls=0, distinct_titles=0,
        top_locations=d.get("location_context") or "", sample_url=roots[0] if roots else "",
        sources=sources, review_status="", jev={}, site_evidence=evs, historical={})


def seed_done(conn) -> int:
    """Pre-seed the 800 wave-001 rows as done in the engine ledger."""
    n = 0
    import datetime
    now = datetime.datetime.now(datetime.timezone.utc).isoformat()
    for b in range(1, 21):
        path = os.path.join(WAVE, f"results-{b:02d}.json")
        if not os.path.exists(path):
            continue
        for rec in json.load(open(path)):
            conn.execute(
                "INSERT OR IGNORE INTO row_state(row_id, stage, result_json, updated_at)"
                " VALUES(?,?,?,?)",
                (rec["source_id"], "done", json.dumps(rec, ensure_ascii=False), now))
            n += 1
    conn.commit()
    return n


def cmd_prepare() -> int:
    cc.make_outdirs(OUTDIR)
    conn = cc.open_ledger(os.path.join(OUTDIR, "ledger.sqlite3"))
    caps = {"global_cap": UNLIMITED, "firstpass_cap": UNLIMITED,
            "followup_cap": UNLIMITED, "audit_cap": UNLIMITED, "version": "search-b-full-1"}
    with conn:
        for k, v in caps.items():
            conn.execute("INSERT OR REPLACE INTO meta(k,v) VALUES(?,?)", (k, str(v)))
    seeded = seed_done(conn)
    wl = load_worklist()
    done = cc.RunState(conn).done_ids()
    remaining = [r for r in wl if r["source_id"] not in done]
    print(f"prepare: {len(wl)} worklist rows; {len(done)} already done; "
          f"{len(remaining)} queued; caps unlimited")
    print(f"prepare: seeded {seeded} wave-001 rows into engine ledger")
    return 0


def build_all() -> tuple:
    # Only Parallel + DDG have working credentials; round-robin across both.
    cc.ADAPTER_CLASSES[:] = [cc.ParallelAdapter, DuckDuckGoAdapter]
    # Throughput: Parallel API tolerates ~6 rps; raise the global search rate and
    # page-attempt cap to match (the run would exhaust 30k fetch attempts otherwise).
    cc.GLOBAL_PAGE_ATTEMPTS = 1_000_000_000
    conn, rs, budget, fetcher, sc, runtime = cc.build_engine(OUTDIR)
    sc.search_rate = cc.RateLimiter(6.0, 8)
    sc.inflight = cc.SharedInflight(8)
    sc.provider_rate["parallel"] = cc.RateLimiter(6.0, 6)
    # DDG is a scraped free lane: hold it to ~0.5 req/s so we stay welcome.
    sc.provider_rate["ddg"] = cc.RateLimiter(0.5, 1)
    wl = load_worklist()
    identities = {r["source_id"]: to_identity_row(r) for r in wl}
    return conn, rs, budget, fetcher, sc, runtime, identities


def cmd_run(limit=None) -> int:
    conn, rs, budget, fetcher, sc, runtime, identities = build_all()
    done = rs.done_ids()
    queue = [identities[r["source_id"]] for r in load_worklist() if r["source_id"] not in done]
    if limit:
        queue = queue[:limit]
    total = len(queue)
    print(f"run: {total} rows to process (6 workers)", flush=True)
    t0 = time.monotonic()
    lock = threading.Lock()
    counter = {"done": 0}
    statuses: dict[str, int] = {}

    def work(ir):
        try:
            rr = cc.process_row(ir.id, ir, sc, fetcher, rs, OUTDIR, runtime)
        except Exception as e:  # noqa: BLE001 - per-row isolation, engine semantics
            rr = cc.finish_untouched(ir.id, ir, rs, OUTDIR, "interrupted", repr(e)[:200])
        rs.set_done(ir.id, rr, OUTDIR)
        return rr

    def parallel_dead() -> bool:
        recent = conn.execute(
            "SELECT provider, outcome FROM search_reservations ORDER BY res_id DESC LIMIT 12"
        ).fetchall()
        auth = sum(1 for r in recent if r["provider"] == "parallel" and r["outcome"] == "auth")
        return auth >= 8

    from concurrent.futures import ThreadPoolExecutor, as_completed
    with ThreadPoolExecutor(max_workers=6) as pool:
        futures = {pool.submit(work, ir): ir for ir in queue}
        stopped = False
        for fut in as_completed(futures):
            rr = fut.result()
            with lock:
                counter["done"] += 1
                statuses[rr.identity_status] = statuses.get(rr.identity_status, 0) + 1
                n = counter["done"]
            if n % 50 == 0 or n == total:
                rate = n / max(time.monotonic() - t0, 1e-6)
                eta_min = (total - n) / max(rate, 1e-6) / 60
                top = sorted(statuses.items(), key=lambda kv: -kv[1])[:6]
                print(f"run: {n}/{total} rows ({rate:.2f} rows/s, eta {eta_min:.0f} min) "
                      f"statuses={top}", flush=True)
                json.dump({"done": n, "total": total, "statuses": statuses,
                           "ts": time.time()},
                          open(os.path.join(OUTDIR, "progress.json"), "w"))
                if parallel_dead():
                    print("PARALLEL-DEAD: Parallel quota exhausted; cancelling remaining "
                          "rows so they can be resumed after a top-up.", flush=True)
                    stopped = True
                    for f in futures:
                        f.cancel()
                    break
    print(f"run: complete — {counter['done']} rows processed"
          f"{' (STOPPED EARLY: PARALLEL-DEAD)' if stopped else ''}", flush=True)
    json.dump({"done": counter["done"], "total": total, "statuses": statuses,
               "complete": not stopped, "ts": time.time()},
              open(os.path.join(OUTDIR, "progress.json"), "w"))
    return 0


def cmd_status() -> int:
    conn = cc.open_ledger(os.path.join(OUTDIR, "ledger.sqlite3"))
    done = cc.RunState(conn).done_ids()
    wl = load_worklist()
    remaining = len(wl) - len(done)
    searches = conn.execute("SELECT COUNT(*) FROM search_reservations").fetchone()[0]
    fetches = conn.execute("SELECT COUNT(*) FROM fetches").fetchone()[0]
    print(f"status: {len(done)} done, {remaining} remaining of {len(wl)}; "
          f"{searches} searches, {fetches} fetches in ledger")
    try:
        print("progress.json:", open(os.path.join(OUTDIR, "progress.json")).read())
    except OSError:
        pass
    return 0


def cmd_followup(limit=None) -> int:
    """Re-process unresolved rows with a location-bearing query variant."""
    conn, rs, budget, fetcher, sc, runtime, identities = build_all()
    UNRESOLVED = {"ambiguous", "researched-no-confirmed-site", "inaccessible",
                  "not-attempted-budget", "not-attempted-time", "interrupted"}
    wl = {r["source_id"]: r for r in load_worklist()}
    targets = []
    for rid, v in rs.all().items():
        if v["stage"] != "done" or not v["result"]:
            continue
        d = json.loads(v["result"])
        if d.get("identity_status") in UNRESOLVED and rid in wl:
            targets.append(rid)
    if limit:
        targets = targets[:limit]
    print(f"followup: {len(targets)} unresolved rows to re-process", flush=True)
    # drop old results so they are reprocessed; keep a backup of prior results
    prior = {rid: json.loads(rs.all()[rid]["result"]) for rid in targets}
    json.dump(prior, open(os.path.join(OUTDIR, "followup-prior-results.json"), "w"))
    with conn:
        for rid in targets:
            conn.execute("DELETE FROM row_state WHERE row_id=?", (rid,))

    def variant_ir(rid: str) -> cc.IdentityRow:
        ir = identities[rid]
        loc = (wl[rid].get("location_context") or "").split(";")[0].strip()
        if loc and loc.lower() not in ir.name.lower():
            base = cc.IdentityRow(**{**ir.__dict__, "name": f"{ir.name} {loc}"})
            return base
        return ir

    t0 = time.monotonic()
    lock = threading.Lock()
    counter = {"done": 0}
    statuses: dict[str, int] = {}

    def work(rid: str):
        ir = variant_ir(rid)
        try:
            rr = cc.process_row(rid, ir, sc, fetcher, rs, OUTDIR, runtime)
        except Exception as e:  # noqa: BLE001
            rr = cc.finish_untouched(rid, ir, rs, OUTDIR, "interrupted", repr(e)[:200])
        rs.set_done(rid, rr, OUTDIR)
        return rr

    from concurrent.futures import ThreadPoolExecutor, as_completed
    with ThreadPoolExecutor(max_workers=6) as pool:
        futures = {pool.submit(work, rid): rid for rid in targets}
        for fut in as_completed(futures):
            rr = fut.result()
            with lock:
                counter["done"] += 1
                statuses[rr.identity_status] = statuses.get(rr.identity_status, 0) + 1
                n = counter["done"]
            if n % 50 == 0 or n == len(targets):
                rate = n / max(time.monotonic() - t0, 1e-6)
                print(f"followup: {n}/{len(targets)} ({rate:.2f} rows/s) "
                      f"statuses={sorted(statuses.items(), key=lambda kv: -kv[1])[:5]}", flush=True)
                json.dump({"phase": "followup", "done": n, "total": len(targets),
                           "statuses": statuses, "ts": time.time()},
                          open(os.path.join(OUTDIR, "progress.json"), "w"))
    print(f"followup: complete — {counter['done']} rows", flush=True)
    return 0


def cmd_merge() -> int:
    conn, rs, *_ = build_all()
    import sqlite3
    done = {}
    for rid, v in rs.all().items():
        if v["stage"] == "done" and v["result"]:
            try:
                done[rid] = json.loads(v["result"])
            except Exception:
                pass
    wl = {r["source_id"]: r for r in load_worklist()}
    out_rows = []
    for rid, d in sorted(done.items()):
        ev = d.get("evidence") or []
        fetch_urls = ";".join(e.get("url", "") for e in ev if e.get("kind") == "fetch")[:800]
        search_urls = ";".join(u for e in ev if e.get("kind") == "search"
                               for u in (e.get("urls") or []))[:800]
        out_rows.append({
            "source_id": rid,
            "observed_name": wl.get(rid, {}).get("observed_name") or d.get("observed_name"),
            "identity_status": d.get("identity_status"),
            "careers_status": d.get("careers_status"),
            "official_site": d.get("official_site"),
            "careers_page": d.get("careers_page"),
            "ats": d.get("ats"),
            "organization_type": d.get("organization_type"),
            "reason_code": d.get("reason_code"),
            "search_attempts": d.get("search_attempts") or 0,
            "fetch_evidence_urls": fetch_urls,
            "search_result_urls": search_urls,
        })
    path = os.path.join(OUTDIR, "merged-results.json")
    json.dump(out_rows, open(path, "w"), indent=1)
    counts: dict[str, int] = {}
    for r in out_rows:
        counts[r["identity_status"]] = counts.get(r["identity_status"], 0) + 1
    print(f"merge: {len(out_rows)} rows written to {path}")
    print("merge: status counts:", json.dumps(counts, sort_keys=True))
    return 0


def main() -> int:
    cmd = sys.argv[1] if len(sys.argv) > 1 else "status"
    if cmd == "prepare":
        return cmd_prepare()
    if cmd == "run":
        lim = int(sys.argv[sys.argv.index("--limit") + 1]) if "--limit" in sys.argv else None
        return cmd_run(lim)
    if cmd == "followup":
        lim = int(sys.argv[sys.argv.index("--limit") + 1]) if "--limit" in sys.argv else None
        return cmd_followup(lim)
    if cmd == "merge":
        return cmd_merge()
    if cmd == "status":
        return cmd_status()
    print(__doc__)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
