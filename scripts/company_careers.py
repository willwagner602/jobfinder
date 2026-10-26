#!/usr/bin/env python3
"""company_careers.py — resumable company/careers resolution pipeline.

Subcommands:
  prepare            snapshot workbook inputs, initialize SQLite ledger + budgets
  pilot              first stratified pilot cohort (shares the same budgets/ledger)
  run [--max-rows N] process more rows in deterministic queue order
  status             progress + budget metrics
  audit [--limit N]  list evidence references for pilot rows needing manual review
  approve-checkpoint --milestone {100,1000} --operator NAME [--report-hash H]
  merge              (v1: guarded — see README; requires operator confirmation)

Budget semantics:
  - Hard global ceiling of 5,000 outbound search attempts across pilot/runs/restarts.
  - A token is reserved transactionally BEFORE dispatch; a crash after reservation
    counts the attempt as spent (no refunds for ambiguous failures).
  - Cache hits on identical (provider, query) cost no new token.
  - Reserves: firstpass 3500, followup 1000, audit 500. Transfers only after the
    corresponding queue is exhausted (v1: manual, documented).
"""
from __future__ import annotations

import argparse
import contextlib
import dataclasses
import datetime
import hashlib
import html.parser
import ipaddress
import json
import os
import random
import re
import socket
import sqlite3
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field
from typing import Any, Callable, Optional

WORKBOOK_DEFAULT = "artifacts/job-digest-mining-report.xlsx"
OUTDIR_DEFAULT = "artifacts/company-careers-v1"
VERSION = "company-careers-v1"

GLOBAL_SEARCH_BUDGET = 5000
FIRSTPASS_RESERVE = 3500
FOLLOWUP_RESERVE = 1000
AUDIT_RESERVE = 500
PASS_ALLOCATION = {
    "firstpass": FIRSTPASS_RESERVE, "followup": FOLLOWUP_RESERVE, "audit": AUDIT_RESERVE,
}
HOST_PASS_TARGETS = ("firstpass", "followup", "audit")
PASS_CAPS = PASS_ALLOCATION

RUN_DEADLINE_SECONDS = 3 * 3600
GOAL_SEARCH_RATE = 2.0
GOAL_SEARCH_INFLIGHT = 6
PER_PROVIDER_RATE = 1.0
PER_PROVIDER_INFLIGHT = 1
FETCH_WORKERS = 8
MAX_PAGES_PER_ROW = 8
PER_HOST_INTERVAL = 1.0
MAX_HOST_CONCURRENT = 2
MAX_REDIRECTS = 4
FETCH_TIMEOUT = 15.0
MAX_RESPONSE_BYTES = 2 * 1024 * 1024
GLOBAL_PAGE_ATTEMPTS = 30000

USER_AGENT = "wb-jobs-research/1.0 (research bot)"

# ===================================================================== utils

def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()

def sha256_text(text: str) -> str:
    return sha256(text.encode("utf-8"))

def now_iso() -> str:
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

def json_write_atomic(path: str, obj: Any) -> None:
    tmp = path + ".tmp"
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(obj, fh, ensure_ascii=False, indent=1, sort_keys=True)
        fh.write("\n")
    os.replace(tmp, path)

def norm_ws(v: Any) -> str:
    return re.sub(r"\s+", " ", str(v)).strip() if v is not None else ""

def int_or0(v) -> int:
    try:
        return max(0, int(v))
    except Exception:
        return 0

def float_or(v, default=0.0) -> float:
    try:
        return float(v)
    except Exception:
        return default

def hash_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()

def load_openpyxl_version():
    try:
        import openpyxl
        return openpyxl.__version__
    except Exception:
        return None

# ================================================================== URL safety

AGGREGATOR_HOSTS = {
    "adzuna.com", "indeed.com", "glassdoor.com", "linkedin.com", "ziprecruiter.com",
    "monster.com", "simplyhired.com", "jooble.org", "jobleads.com", "climatebase.org",
    "serpapi.com", "news.ycombinator.com",
}

def _suffix_matches(host: str, domains: set[str]) -> bool:
    host = (host or "").lower().rstrip(".")
    parts = host.split(".")
    for n in range(2, len(parts) + 1):
        if ".".join(parts[-n:]) in domains:
            return True
    return False

def is_aggregator_host(host: str) -> bool:
    if not host:
        return False
    return _suffix_matches(host, AGGREGATOR_HOSTS)

class UnsafeURLError(Exception):
    pass

_LOCAL_V4 = [ipaddress.ip_network(n) for n in (
    "0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
    "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24", "192.168.0.0/16",
    "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
    "255.255.255.255/32",
)]

def is_local_ip(ip: str) -> bool:
    try:
        addr = ipaddress.ip_address(ip)
    except ValueError:
        return True
    if addr.version == 4:
        return any(addr in net for net in _LOCAL_V4)
    return (addr.is_loopback or addr.is_link_local or addr.is_multicast
            or addr.is_private or addr.packed[0] in (0xfc, 0xfd))

    try:
        return urllib.parse.urlsplit(u).path in ("", "/")
    except ValueError:
        return True

def is_ip_literal(host: str) -> bool:
    try:
        ipaddress.ip_address(host)
        return True
    except ValueError:
        return False

def looks_like_root_only(u: str) -> bool:
    try:
        return urllib.parse.urlsplit(u).path in ("", "/")
    except ValueError:
        return True

def resolve_ips_safe(host: str, resolver: Optional[Callable] = None) -> list[str]:
    """Resolve DNS; raise UnsafeURLError on private/reserved targets (anti-rebinding)."""
    getinfo = resolver or socket.getaddrinfo
    try:
        infos = getinfo(host, None) if resolver is None else getinfo(host)
    except OSError as exc:
        raise UnsafeURLError(f"DNS resolution failed for {host!r}: {exc}")
    ips = []
    for info in infos:
        ip = info[4][0] if resolver is None else (info[0] if isinstance(info, tuple) else info)
        ip = str(ip)
        if is_local_ip(ip):
            raise UnsafeURLError(f"host {host!r} resolves to local/reserved IP {ip}")
        ips.append(ip)
    if not ips:
        raise UnsafeURLError(f"host {host!r} resolved to no addresses")
    return ips

def validate_public_url(u: str) -> str:
    parsed = urllib.parse.urlsplit(u)
    if parsed.scheme not in ("http", "https"):
        raise UnsafeURLError(f"non-http(s) scheme in {u!r}")
    if not parsed.hostname:
        raise UnsafeURLError(f"missing hostname in {u!r}")
    host = parsed.hostname
    if is_ip_literal(host):
        if is_local_ip(host):
            raise UnsafeURLError(f"local IP literal: {u}")
        return host
    if is_aggregator_host(host):
        raise UnsafeURLError(f"aggregator host blocked from page fetch: {u}")
    return host

def url_host(u: str) -> str:
    try:
        return (urllib.parse.urlsplit(u).hostname or "").lower()
    except ValueError:
        return ""

def root_domain_of_host(host: str) -> str:
    host = (host or "").lower()
    labels = [l for l in host.split(".") if l]
    return ".".join(labels[-2:]) if labels else host

def norm_url(u: str) -> str:
    try:
        p = urllib.parse.urlsplit((u or "").strip())
        return urllib.parse.urlunsplit((p.scheme.lower(), p.netloc.lower(), p.path.rstrip("/"), "", ""))
    except ValueError:
        return (u or "").strip()

def looks_like_careers_link(u: str) -> bool:
    l = (u or "").lower()
    if not l or l.startswith(("mailto:", "javascript:", "tel:", "#")):
        return False
    if any(x in l for x in ("facebook.com", "instagram.com", "twitter.com", "youtube.com",
                            "tiktok.com", ".png", ".jpg", ".jpeg", ".gif", ".css", ".ico",
                            "javascript:", "mailto:", "tel:")):
        return False
    return bool(re.search(r"career|jobs|join[-_ ]?us|work[-_ ]?with[-_ ]?us|open[-_ ]?positions|vacanc|hiring|opportunit|karriere|recruit", l))

ATS_PROVIDERS = ("smartrecruiters.com", "greenhouse.io", "ashbyhq.com", "myworkdayjobs.com",
                 "workable.com", "jobvite.com", "recruitee.com", "teamtailor.com", "lever.co",
                 "taleo.net", "icims.com", "paylocity.com", "jobadder.com", "pinpoint.so",
                 "rippling-ats.com", "bamboohr.com", "jazzhr.com", "applytojob.com")

def ats_provider_of(u: str) -> Optional[str]:
    h = url_host(u) or ""
    for p in ATS_PROVIDERS:
        if p in h:
            return p
    return None

# =================================================================== snapshot

SHEET_IDENTITY = "1 - Company Identity"
SHEET_EVIDENCE = "2 - Site Evidence"
SHEET_JEV = "5 - Jev Preliminary"
SHEET_VERIFY = "3 - Site Verification"
SHEET_POOL = "4 - 15k Pool Outcome"
SHEET_CSITES = "Company Sites"
REQUIRED_SHEETS = [SHEET_IDENTITY, SHEET_EVIDENCE, SHEET_JEV, SHEET_VERIFY, SHEET_POOL, SHEET_CSITES]

URL_CLASS_KIND = {
    "Aggregator": "discovery-only",
    "Company domain": "careers-site-lead",
    "ATS board": "ats-lead",
    "Unclassified": "third-party-lead",
    "Malformed": "discard",
}

@dataclass
class EvidenceUrl:
    url: str
    url_class: str
    role: str
    kind: str
    distinct_urls: int
    samples: list[str]
    sources: str

@dataclass
class IdentityRow:
    id: str
    name: str
    name_variants: str
    urls: int
    hn_urls: int
    distinct_titles: int
    top_locations: str
    sample_url: str
    sources: str
    review_status: str
    jev: dict = field(default_factory=dict)
    site_evidence: list[EvidenceUrl] = field(default_factory=list)
    historical: dict = field(default_factory=dict)

    @property
    def route(self) -> str:
        raw = ((self.jev or {}).get("route_raw") or "").lower()
        if "defer" in raw: return "defer"
        if "likely" in raw: return "likely"
        return "uncertain"

    def name_candidates(self) -> list[str]:
        cands = [self.name]
        for v in re.split(r"[;|]", self.name_variants or ""):
            nv = v.strip()
            if nv and nv not in cands:
                cands.append(nv)
        return [c for c in cands if c]

def read_sheet(wb, name: str) -> tuple[list[str], list[dict]]:
    import openpyxl
    if name not in wb.sheetnames:
        raise SystemExit(f"preflight: required sheet missing: {name!r}")
    ws = wb[name]
    headers = [str(c) if c is not None else "" for c in
               next(ws.iter_rows(min_row=1, max_row=1, values_only=True))]
    rows = []
    for row in ws.iter_rows(min_row=2, values_only=True):
        if row[0] is None:
            continue
        rows.append({headers[i]: row[i] for i in range(min(len(headers), len(row)))})
    return headers, rows

EXPECTED_HEADERS = {
    SHEET_IDENTITY: ["Report company ID", "Observed company name", "Sample job URL"],
    SHEET_EVIDENCE: ["Report company ID", "URL class", "URL / board root",
                     "Sample full URLs (up to 3)"],
    SHEET_JEV: ["Report company ID", "Provisional research routing (not verified)"],
    SHEET_POOL: ["Report company ID"],
    SHEET_VERIFY: ["Company"],
    SHEET_CSITES: ["Company name (candidate)"],
}

def snapshot_inputs(workbook_path: str, outdir: str) -> dict:
    os.makedirs(outdir, exist_ok=True)
    import openpyxl
    wb = openpyxl.load_workbook(workbook_path, read_only=True, data_only=False)
    sheets: dict[str, tuple[list[str], list[dict]]] = {}
    for name in REQUIRED_SHEETS:
        hdr, rows = read_sheet(wb, name)
        for required_col in EXPECTED_HEADERS.get(name, []):
            if required_col not in hdr:
                raise SystemExit(f"preflight: sheet {name!r} missing header {required_col!r}")
        sheets[name] = (hdr, rows)
    rows_id = sheets[SHEET_IDENTITY][1]
    rows_ev = sheets[SHEET_EVIDENCE][1]
    rows_jev = sheets[SHEET_JEV][1]

    identities: dict[str, IdentityRow] = {}
    for r in rows_id:
        tid = str(r["Report company ID"]).strip()
        if not tid:
            raise SystemExit("identity sheet has nonblank row with blank ID")
        if tid in identities:
            raise SystemExit(f"duplicate source ID in identity sheet: {tid}")
        identities[tid] = IdentityRow(
            id=tid, name=norm_ws(r["Observed company name"]),
            name_variants=norm_ws(r.get("Observed name variants")),
            urls=int_or0(r.get("Unique job URLs")), hn_urls=int_or0(r.get("HN URLs")),
            distinct_titles=int_or0(r.get("Distinct raw titles")),
            top_locations=norm_ws(r.get("Top locations")),
            sample_url=str(r.get("Sample job URL") or ""),
            sources=norm_ws(r.get("Sources")),
            review_status=norm_ws(r.get("Review status")) or "Not reviewed")

    for r in rows_ev:
        tid = str(r.get("Report company ID", "") or "").strip()
        if tid not in identities:
            raise SystemExit(f"site-evidence row has unknown ID {tid!r}")
        cls = norm_ws(r["URL class"])
        kind = URL_CLASS_KIND.get(cls)
        if kind is None:
            raise SystemExit(f"unrecognized URL class value: {cls!r}")
        root = str(r["URL / board root"] or "").strip()
        samples_raw = str(r["Sample full URLs (up to 3)"] or "")
        samples = [s.strip() for s in samples_raw.split(";") if s.strip()]
        identities[tid].site_evidence.append(EvidenceUrl(
            url=root, url_class=cls, role=norm_ws(r["Role"]), kind=kind,
            distinct_urls=int_or0(r.get("Distinct URLs")), samples=samples,
            sources=norm_ws(r.get("Sources"))))

    routing = {"likely": 0, "uncertain": 0, "defer": 0}
    for r in rows_jev:
        tid = str(r["Report company ID"]).strip()
        if tid not in identities:
            raise SystemExit(f"jev row has unknown ID {tid!r}")
        route = norm_ws(r["Provisional research routing (not verified)"])
        identities[tid].jev = {
            "route_raw": route,
            "real_p": r.get("Estimated real organization probability"),
            "junk_p": r.get("Estimated junk name probability"),
            "staffing_p": r.get("Estimated staffing intermediary probability"),
            "model": norm_ws(r.get("Model")),
        }
        routing[identities[tid].route] += 1

    rejected_leads: list[dict] = []
    by_name: dict[str, list[str]] = {}
    for tid, ir in identities.items():
        by_name.setdefault(ir.name.strip(), []).append(tid)
    for sheet in (SHEET_POOL, SHEET_VERIFY, SHEET_CSITES):
        hdr, rows_h = sheets[sheet]
        key_col = ("Report company ID" if sheet == SHEET_POOL
                   else "Company" if sheet == SHEET_VERIFY
                   else "Company name (candidate)")
        if key_col not in hdr:
            raise SystemExit(f"preflight: sheet {sheet!r} missing header {key_col!r}")
        for r in rows_h:
            key = str(r.get(key_col) or "").strip()
            if not key:
                continue
            if key_col == "Report company ID":
                if key in identities:
                    identities[key].historical.setdefault(sheet, []).append(r)
                else:
                    rejected_leads.append({"sheet": sheet, "key": key, "reason": "unknown/absent ID"})
                continue
            ids = by_name.get(key, [])
            if not ids:
                rejected_leads.append({"sheet": sheet, "key": key,
                                       "reason": "no exact name match to an ID"})
            elif len(ids) > 1:
                rejected_leads.append({"sheet": sheet, "key": key,
                                       "reason": f"name matches {len(ids)} IDs; ambiguous, not auto-mapped"})
            else:
                identities[ids[0]].historical.setdefault(sheet, []).append(r)

    snap = {
        "version": VERSION,
        "created": now_iso(),
        "workbook": workbook_path,
        "workbook_sha256": hash_file(workbook_path),
        "python": sys.version.split()[0],
        "openpyxl_version": load_openpyxl_version(),
        "counts": {
            "identity_rows": len(identities),
            "evidence_urls": sum(len(r.site_evidence) for r in identities.values()),
            "evidence_ids": sum(1 for r in identities.values() if r.site_evidence),
            "routing": routing,
            "historical_ids": {s: sum(1 for r in identities.values() if s in r.historical)
                               for s in (SHEET_POOL, SHEET_VERIFY, SHEET_CSITES)},
            "rejected_leads": len(rejected_leads),
        },
        "identities": {tid: _identity_dump(ir) for tid, ir in sorted(identities.items())},
        "rejected_leads": rejected_leads,
    }
    json_write_atomic(os.path.join(outdir, "snapshot.json"), snap)
    print(f"snapshot: {len(identities)} identity rows; routing {routing}; "
          f"{len(rejected_leads)} rejected historical leads recorded in snapshot.json")
    wb.close()
    return snap

def _identity_dump(ir: IdentityRow) -> dict:
    return {
        "id": ir.id, "name": ir.name, "name_variants": ir.name_variants,
        "urls": ir.urls, "hn_urls": ir.hn_urls, "distinct_titles": ir.distinct_titles,
        "top_locations": ir.top_locations, "sample_url": ir.sample_url,
        "sources": ir.sources, "review_status": ir.review_status,
        "jev": ir.jev,
        "site_evidence": [dataclasses.asdict(e) for e in ir.site_evidence],
        "historical": ir.historical,
    }

def load_snapshot(outdir: str) -> dict:
    with open(os.path.join(outdir, "snapshot.json")) as fh:
        return json.load(fh)

def load_identity_rows(outdir: str) -> dict[str, IdentityRow]:
    snap = load_snapshot(outdir)
    out = {}
    for tid, d in snap["identities"].items():
        evs = [EvidenceUrl(r["url"], r["url_class"], r["role"], r["kind"],
                           r["distinct_urls"], r["samples"], r["sources"])
               for r in d["site_evidence"]]
        out[tid] = IdentityRow(d["id"], d["name"], d["name_variants"], d["urls"], d["hn_urls"],
                               d["distinct_titles"], d["top_locations"], d["sample_url"],
                               d["sources"], d["review_status"], d.get("jev", {}),
                               evs, d.get("historical", {}))
    return out

# ==================================================================== ledger

LEDGER_SCHEMA = """
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT);
CREATE TABLE IF NOT EXISTS search_reservations (
  res_id INTEGER PRIMARY KEY AUTOINCREMENT,
  row_id TEXT, purpose TEXT, provider TEXT, query TEXT,
  reserved_at TEXT, outcome TEXT, http_status INTEGER, elapsed_ms INTEGER,
  cache_hit INTEGER DEFAULT 0, result_count INTEGER DEFAULT 0, error TEXT,
  consumed INTEGER DEFAULT 1
);
CREATE TABLE IF NOT EXISTS fetches (
  fetch_id INTEGER PRIMARY KEY AUTOINCREMENT,
  row_id TEXT, url TEXT, reserved_at TEXT, outcome TEXT, http_status INTEGER,
  final_url TEXT, sha256 TEXT, text_sha256 TEXT, elapsed_ms INTEGER,
  error TEXT, cached INTEGER DEFAULT 0
);
CREATE TABLE IF NOT EXISTS row_state (
  row_id TEXT PRIMARY KEY,
  stage TEXT,
  result_json TEXT,
  updated_at TEXT
);
CREATE TABLE IF NOT EXISTS approvals (
  milestone TEXT PRIMARY KEY, report_hash TEXT, approved_at TEXT, source TEXT
);
CREATE TABLE IF NOT EXISTS schedule (
  seq INTEGER PRIMARY KEY AUTOINCREMENT, row_id TEXT, pass_kind TEXT, route TEXT,
  priority REAL, scheduled_at TEXT
);
"""

def open_ledger(path: str) -> sqlite3.Connection:
    conn = sqlite3.connect(path, isolation_level=None, timeout=60, check_same_thread=False)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    conn.executescript(LEDGER_SCHEMA)
    return conn

# ------------------------------------------------------------------- budgets

class Budget:
    def __init__(self, conn: sqlite3.Connection):
        self.conn = conn
        self.lock = threading.RLock()
        self._reload()

    def _reload(self):
        m = dict(self.conn.execute("SELECT k,v FROM meta"))
        self.global_cap = int(m.get("global_cap") or GLOBAL_SEARCH_BUDGET)
        self.firstpass_cap = int(m.get("firstpass_cap") or FIRSTPASS_RESERVE)
        self.followup_cap = int(m.get("followup_cap") or FOLLOWUP_RESERVE)
        self.audit_cap = int(m.get("audit_cap") or AUDIT_RESERVE)

    def reserve(self, row_id: str, purpose: str, provider: str, query: str) -> Optional[int]:
        """Reserve one token BEFORE dispatch; None means blocked (global or pass cap)."""
        with self.lock:
            s = self._spent_by_purpose()
            if s["__total__"] >= self.global_cap:
                return None
            cap_key = {"firstpass": self.firstpass_cap, "followup": self.followup_cap,
                       "audit": self.audit_cap}.get(purpose)
            if cap_key and s.get(purpose, 0) >= cap_key:
                return None
            cur = self.conn.execute(
                "INSERT INTO search_reservations(row_id,purpose,provider,query,reserved_at)"
                " VALUES(?,?,?,?,?)", (row_id, purpose, provider, query, now_iso()))
            return cur.lastrowid

    def record(self, res_id: Optional[int], outcome: str, http_status=None, elapsed_ms=None,
               cache_hit=False, result_count=0, error=None):
        if res_id is None:
            return
        with self.lock:
            self.conn.execute(
                "UPDATE search_reservations SET outcome=?, http_status=?, elapsed_ms=?,"
                " cache_hit=?, result_count=?, error=? WHERE res_id=?",
                (outcome, http_status, elapsed_ms, 1 if cache_hit else 0, result_count,
                 (error or "")[:500], res_id))

    def _spent_by_purpose(self) -> dict[str, int]:
        d = {r["purpose"] or "other": r["c"] for r in self.conn.execute(
            "SELECT purpose, COUNT(*) c FROM search_reservations"
            " WHERE consumed=1 GROUP BY purpose")}
        d["__total__"] = self.conn.execute(
            "SELECT COUNT(*) FROM search_reservations WHERE consumed=1").fetchone()[0]
        return d

    def stats(self) -> dict:
        with self.lock:
            s = self._spent_by_purpose()
            self._reload()
            return {
                "global_cap": self.global_cap,
                "consumed_total": s["__total__"],
                "remaining_global": max(0, self.global_cap - s["__total__"]),
                "firstpass": {"spent": s.get("firstpass", 0), "cap": self.firstpass_cap},
                "followup": {"spent": s.get("followup", 0), "cap": self.followup_cap},
                "audit": {"spent": s.get("audit", 0), "cap": self.audit_cap},
            }

# =============================================================== rate limiting

class RateLimiter:
    """Steady-rate limiter with bounded inflight (scope-aware)."""
    def __init__(self, rate: float, inflight: int = 99):
        self.interval = 1.0 / rate if rate > 0 else 0.0
        self.next_at = 0.0
        self.lock = threading.Lock()
        self.sema = threading.Semaphore(inflight)

    @contextlib.contextmanager
    def acquire(self):
        with self.sema:
            with self.lock:
                now = time.monotonic()
                wait = max(0.0, self.next_at - now)
                self.next_at = max(self.next_at, now) + self.interval
            if wait > 0:
                time.sleep(wait)
            yield

class HostGate:
    """Per-host fetch pacing + max concurrent fetches per host."""
    def __init__(self, max_concurrent: int = MAX_HOST_CONCURRENT,
                 min_interval: float = PER_HOST_INTERVAL):
        self.max_concurrent = max_concurrent
        self.min_interval = min_interval
        self.semas: dict[str, threading.Semaphore] = {}
        self.next_free: dict[str, float] = {}
        self.lock = threading.Lock()

    def _sema_for(self, host: str) -> threading.Semaphore:
        with self.lock:
            if host not in self.semas:
                self.semas[host] = threading.Semaphore(self.max_concurrent)
            return self.semas[host]

    @contextlib.contextmanager
    def acquire(self, host: str):
        s = self._sema_for(host)
        with s:
            with self.lock:
                now = time.monotonic()
                wait = max(0.0, self.next_free.get(host, 0) - now)
                self.next_free[host] = max(self.next_free.get(host, 0), now) + self.min_interval
            if wait > 0:
                time.sleep(wait)
            yield

class SharedInflight:
    def __init__(self, limit: int):
        self.sema = threading.Semaphore(limit)
        self.lock = threading.Lock()
        self.inflight = 0

    @contextlib.contextmanager
    def acquire(self):
        with self.sema:
            with self.lock:
                self.inflight += 1
            try:
                yield
            finally:
                with self.lock:
                    self.inflight -= 1

# ============================================================ search adapters

@dataclass
class SearchResult:
    provider: str
    url: str
    title: str
    snippet: str
    rank: int
    raw: dict

class ProviderError(Exception):
    def __init__(self, kind: str, message: str, status: int | None = None,
                 retry_after: int | None = None):
        super().__init__(f"{kind}: {message}")
        self.kind = kind
        self.status = status
        self.retry_after = retry_after

HttpImpl = Callable  # http_impl(method, url, headers, body, timeout) -> (status, bytes)

def _parse_json_or_err(raw: bytes) -> dict:
    try:
        return json.loads(raw.decode("utf-8", "replace"))
    except Exception as e:
        raise ProviderError("api_error", f"unparsable JSON response: {e}")

class SearchAdapter:
    name = "abstract"
    env_key = "MISSING"
    def __init__(self, api_key: Optional[str]):
        self.api_key = api_key
    def available(self) -> bool:
        return bool(self.api_key)
    def search(self, query: str, count: int, http_impl: HttpImpl) -> list[SearchResult]:
        raise NotImplementedError
    def _resp(self, http_impl, method, url, headers, body=None):
        status, raw = http_impl(method, url, headers, body, timeout=FETCH_TIMEOUT)
        js = _parse_json_or_err(raw)
        return status, js
    def _check_status(self, status: int, js: dict,
                      retry_after: int | None = None):
        if status == 200:
            return
        kind = {401: "auth", 402: "auth", 403: "auth", 429: "rate", 529: "rate"}.get(
            status, "server" if status >= 500 else "client")
        msg = (js or {}).get("error") or (js or {}).get("message") or str(js)[:200]
        raise ProviderError(kind, str(msg), status, retry_after)

class BraveAdapter(SearchAdapter):
    name = "brave"
    env_key = "BRAVE_API_KEY"
    def search(self, query, count=5, http_impl=None):
        url = "https://api.search.brave.com/res/v1/web/search?" + urllib.parse.urlencode(
            {"q": query, "count": count})
        status, js = self._resp(http_impl, "GET", url,
            {"X-Subscription-Token": self.api_key, "Accept": "application/json"})
        self._check_status(status, js)
        return [SearchResult(self.name, r.get("url",""), r.get("title",""),
                             r.get("description") or r.get("snippet") or "", i + 1,
                             {"_raw": r})
                for i, r in enumerate(((js.get("web") or {}).get("results") or [])[:count])]

class ExaAdapter(SearchAdapter):
    name = "exa"
    env_key = "EXA_API_KEY"
    def search(self, query, count=5, http_impl=None):
        status, js = self._resp(http_impl, "POST", "https://api.exa.ai/search",
            {"x-api-key": self.api_key, "Content-Type": "application/json"},
            {"query": query, "numResults": count, "type": "auto"})
        self._check_status(status, js)
        out = []
        for i, r in enumerate((js.get("results") or [])[:count]):
            snip = ""
            if r.get("highlights"):
                snip = " | ".join(str(h) for h in r["highlights"][:2])
            elif r.get("text"):
                snip = str(r["text"])[:300]
            out.append(SearchResult(self.name, r.get("url",""), r.get("title",""), snip, i + 1,
                {"_raw": r, "costDollars": js.get("costDollars")}))
        return out

class TavilyAdapter(SearchAdapter):
    name = "tavily"
    env_key = "TAVILY_API_KEY"
    def search(self, query, count=5, http_impl=None):
        body = {"query": query, "max_results": count, "search_depth": "basic",
                "auto_parameters": False, "include_answer": False,
                "include_raw_content": False, "include_usage": True}
        status, js = self._resp(http_impl, "POST", "https://api.tavily.com/search",
            {"Authorization": "Bearer " + self.api_key, "Content-Type": "application/json"},
            body)
        self._check_status(status, js)
        return [SearchResult(self.name, r.get("url",""), r.get("title",""),
                             r.get("content",""), i + 1,
                             {"_raw": r, "usage": js.get("usage")})
                for i, r in enumerate((js.get("results") or [])[:count])]

class KagiAdapter(SearchAdapter):
    name = "kagi"
    env_key = "KAGI_API_KEY"
    def search(self, query, count=5, http_impl=None):
        status, js = self._resp(http_impl, "GET",
            "https://kagi.com/api/v1/search?q=" + urllib.parse.quote(query),
            {"Authorization": "Bot " + self.api_key, "Accept": "application/json"})
        self._check_status(status, js)
        if js.get("error"):
            raise ProviderError("api_error", f"HTTP 200 with body error: {js.get('error')}", 200)
        items = [it for it in (js.get("data") or []) if it.get("t") == 0][:count]
        return [SearchResult(self.name, it.get("url",""), it.get("title",""),
                             it.get("snippet",""), i + 1,
                             {"_raw": {"t": it.get("t")},
                              "api_balance": (js.get("meta") or {}).get("api_balance")})
                for i, it in enumerate(items)]

class ParallelAdapter(SearchAdapter):
    name = "parallel"
    env_key = "PARALLEL_API_KEY"
    def search(self, query, count=5, http_impl=None):
        body = {"search_queries": [query], "mode": "fast",
                "advanced_settings": {"max_results": count}}
        status, js = self._resp(http_impl, "POST", "https://api.parallel.ai/v1/search",
            {"x-api-key": self.api_key, "Content-Type": "application/json"}, body)
        self._check_status(status, js)
        out = []
        for i, r in enumerate((js.get("results") or [])[:count]):
            exc = r.get("excerpts") or []
            out.append(SearchResult(self.name, r.get("url",""), r.get("title",""),
                                    (" | ".join(str(x) for x in exc[:2]))[:400], i + 1,
                                    {"_raw": r, "usage": js.get("usage")}))
        return out

class YouAdapter(SearchAdapter):
    name = "you"
    env_key = "YOU_API_KEY"
    def search(self, query, count=5, http_impl=None):
        status, js = self._resp(http_impl, "POST", "https://ydc-index.io/v1/search",
            {"X-API-Key": self.api_key, "Content-Type": "application/json"},
            {"query": query, "count": count})
        self._check_status(status, js)
        out = []
        web = (js.get("results") or {}).get("web") if isinstance(js.get("results"), dict) else None
        for i, r in enumerate((web or [])[:count]):
            snip = r.get("description") or (" | ".join(r.get("snippets") or [] [:2]))
            out.append(SearchResult(self.name, r.get("url",""), r.get("title",""), snip or "",
                                    i + 1, {"_raw": r}))
        return out

ADAPTER_CLASSES = [BraveAdapter, ExaAdapter, TavilyAdapter, KagiAdapter, ParallelAdapter, YouAdapter]

def build_adapters(env) -> list[SearchAdapter]:
    return [cls(env.get(cls.env_key)) for cls in ADAPTER_CLASSES]

# ================================================================= fetch pool

class FetchOutcome:
    __slots__ = ("url", "final_url", "status", "sha256", "text", "links", "error",
                 "cached", "elapsed_ms", "title")
    def __init__(self, url, final_url, status, sha, text, links, error, cached=False,
                 elapsed_ms=None):
        self.url = url; self.final_url = final_url; self.status = status
        self.sha256 = sha; self.text = text or ""; self.links = list(links or [])
        self.error = error; self.cached = cached; self.elapsed_ms = elapsed_ms
        self.title = self.text.split(" :: ", 1)[0] if " :: " in self.text else ""
    def as_dict(self):
        return {"url": self.url, "final_url": self.final_url, "status": self.status,
                "sha256": self.sha256, "text": self.text[:3000], "links": self.links[:60],
                "error": self.error, "cached": self.cached, "elapsed_ms": self.elapsed_ms,
                "title": self.title}
    def as_evidence(self):
        return {"kind": "fetch", "url": self.url, "final_url": self.final_url,
                "status": self.status, "sha256": self.sha256, "cached": self.cached}

def copy_cached(fo: FetchOutcome) -> FetchOutcome:
    return FetchOutcome(fo.url, fo.final_url, fo.status, fo.sha256, fo.text, fo.links,
                        fo.error, True, fo.elapsed_ms)

def _urllib_send(url: str, timeout: float = FETCH_TIMEOUT):
    req = urllib.request.Request(url, headers={
        "User-Agent": USER_AGENT,
        "Accept": "text/html,application/xhtml+xml,*/*;q=0.8",
        "Accept-Language": "en-US,en;q=0.8"})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            data = resp.read(MAX_RESPONSE_BYTES + 1)
            if len(data) > MAX_RESPONSE_BYTES:
                return None, b"", url, f"response-too-large:{len(data)}"
            return resp.status, data, resp.geturl(), None
    except urllib.error.HTTPError as e:
        return e.code, b"", url, None
    except Exception as e:
        return None, b"", url, f"{type(e).__name__}: {e}"

def parse_html(raw: bytes, base_url: str) -> tuple[str, list[str], list[dict]]:
    class P(html.parser.HTMLParser):
        def __init__(self):
            super().__init__(convert_charrefs=True)
            self.title: list[str] = []
            self.text: list[str] = []
            self.links: list[str] = []
            self.jsonld: list[dict] = []
            self._in_title = 0
            self._skip = 0
            self._in_jsonld = 0
        def handle_starttag(self, tag, attrs):
            d = dict(attrs)
            if tag == "title": self._in_title += 1
            if tag in ("script", "style", "svg", "iframe", "noscript"): self._skip += 1
            if tag == "script" and (d.get("type") or "") == "application/ld+json":
                self._in_jsonld += 1
            if tag == "a" and d.get("href"):
                try:
                    self.links.append(urllib.parse.urljoin(base_url, d["href"]))
                except ValueError:
                    pass
        def handle_endtag(self, tag):
            if tag == "title" and self._in_title: self._in_title -= 1
            if tag in ("script", "style", "svg", "iframe", "noscript") and self._skip:
                self._skip -= 1
            if tag == "script" and self._in_jsonld: self._in_jsonld -= 1
        def handle_data(self, data):
            if self._in_title: self.title.append(data)
            if self._in_jsonld:
                try:
                    self.jsonld.append(json.loads(data))
                except Exception:
                    pass
                return
            if self._skip: return
            self.text.append(data)
    if not raw:
        return "", [], []
    p = P()
    try:
        p.feed(raw.decode("utf-8", "replace"))
    except Exception:
        pass
    title = norm_ws(" ".join(p.title))
    text = norm_ws(" ".join(p.text))[:20000]
    orgnames = []
    for js in p.jsonld[:4]:
        nodes = js.get("@graph") if isinstance(js, dict) and isinstance(js.get("@graph"), list) else [js]
        for node in nodes:
            if isinstance(node, dict):
                n = node.get("name")
                if isinstance(n, str):
                    orgnames.append(n)
    return title + " :: " + text, sorted(set(p.links)), orgnames

class PageFetcher:
    """Thread-safe fetch layer with cache, per-host pacing, page-attempt accounting,
    DNS rebinding guard and response size cap."""
    def __init__(self, conn: sqlite3.Connection, outdir: str, page_counter: dict,
                 host_gate: Optional[HostGate] = None,
                 allowed_test_hosts: Optional[set[str]] = None):
        self.conn = conn
        self.lock = threading.RLock()
        self.host_gate = host_gate or HostGate()
        self.page_counter = page_counter
        self.allowed_test_hosts = allowed_test_hosts or set()
        self.evidence_dir = os.path.join(outdir, "evidence", "http")
        os.makedirs(self.evidence_dir, exist_ok=True)
        self._cache: dict[str, FetchOutcome] = {}

    def _page_attempt(self) -> bool:
        with self.lock:
            if self.page_counter["attempts"] >= GLOBAL_PAGE_ATTEMPTS:
                return False
            self.page_counter["attempts"] += 1
            return True

    def fetch(self, url: str, row_id: str = "") -> FetchOutcome:
        key = norm_url(url)
        with self.lock:
            hit = self._cache.get(key)
        if hit is not None:
            return copy_cached(hit)
        try:
            host = validate_public_url(url)
            if host not in self.allowed_test_hosts and not is_ip_literal(host):
                resolve_ips_safe(host)
        except UnsafeURLError as e:
            return FetchOutcome(url, url, None, None, "", [], f"unsafe-url: {e}")
        if not self._page_attempt():
            return FetchOutcome(url, url, None, None, "", [], "global-page-attempt-cap")
        with self.host_gate.acquire(host):
            t0 = time.monotonic()
            status, raw, final_url, error = _urllib_send(url)
            elapsed = int((time.monotonic() - t0) * 1000)
        if not error and raw is not None and len(raw) > MAX_RESPONSE_BYTES:
            status, raw, error = None, b"", f"response-too-large:{len(raw)}"
        if error:
            fo = FetchOutcome(url, url, None, None, "", [], error, False, elapsed)
        elif status == 200:
            text, links, orgnames = parse_html(raw, final_url or url)
            fo = FetchOutcome(url, final_url, status, sha256(raw or b""), text, links, None,
                              False, elapsed)
            self._save_http_evidence(self.evidence_dir, final_url or url, raw)
        else:
            fo = FetchOutcome(url, url, status, sha256(raw or b""), "", [], None, False, elapsed)
        with self.lock:
            self._cache[key] = fo
            self.conn.execute(
                "INSERT INTO fetches(row_id, url, reserved_at, outcome, http_status, final_url,"
                " sha256, text_sha256, elapsed_ms, error, cached) VALUES(?,?,?,?,?,?,?,?,?,?,?)",
                (row_id, url, now_iso(),
                 "error" if fo.error else ("page" if fo.status == 200 else "http-error"),
                 fo.status, fo.final_url, fo.sha256, sha256_text(fo.text), fo.elapsed_ms,
                 fo.error, 0))
            self.conn.commit()
        return fo

    get = fetch  # alias

    def _save_http_evidence(self, directory: str, url: str, raw: bytes):
        """Persist raw HTTP body under evidence/http, addressed by a hash of the URL."""
        if not raw:
            return
        name = sha256_text(url)[:24] + ".html"
        path = os.path.join(directory, name)
        if not os.path.exists(path):
            try:
                with open(path, "wb") as fh:
                    fh.write(raw)
            except OSError:
                pass

# =============================================================== result types

@dataclass
class RowResult:
    source_id: str
    observed_name: str
    proposed_canonical_name: Optional[str] = None
    canonical_status: str = "unknown"
    organization_type: Optional[str] = None
    official_site: Optional[str] = None
    careers_page: Optional[str] = None
    listing_source: Optional[str] = None
    ats: Optional[str] = None
    identity_status: str = "not-attempted-budget"
    careers_status: Optional[str] = None
    reason_code: str = ""
    evidence: list[dict] = field(default_factory=list)
    request_ids: dict = field(default_factory=dict)
    search_attempts: int = 0
    confidence_note: Optional[str] = None
    fetched_at: str = ""
    route: str = ""

    def to_record(self) -> dict:
        return {f: getattr(self, f) for f in (
            "source_id","observed_name","proposed_canonical_name","canonical_status",
            "organization_type","official_site","careers_page","listing_source","ats",
            "identity_status","careers_status","reason_code","evidence","request_ids",
            "search_attempts","confidence_note","fetched_at","route")}

OUTCOME_STATUSES = {
    "verified": ("verified-official-identity", "verified-official-careers",
                 "verified-employer-branded-board"),
    "resolved": ("verified-official-identity", "verified-official-careers",
                 "verified-employer-branded-board", "candidate", "ambiguous",
                 "researched-no-confirmed-site"),
    "untouched": ("untouched", "not-attempted-budget", "not-attempted-time", "interrupted",
                  "blocked-by-operator"),
}

def is_terminal_status(s: str) -> bool:
    return (s in OUTCOME_STATUSES["resolved"]
            or s in OUTCOME_STATUSES["untouched"]
            or s in ("inaccessible",))

def search_that_status_counts(s: str) -> bool:
    """Distinct-processed-row counting: statuses reachable through actual research work."""
    return s in OUTCOME_STATUSES["resolved"] or s in ("inaccessible",)

# =========================================================== promotion gates

def first_name_present(name_candidates: list[str], text_lower: str) -> Optional[str]:
    for v in name_candidates:
        vl = v.lower().strip()
        if vl and vl in text_lower:
            return v
    return None

def check_identity_gate(ir: IdentityRow, fo: FetchOutcome,
                        identity_score: Optional[float],
                        conflict_score: Optional[float]) -> dict:
    """Identity-only promotion gate. Snippets, homepages-only text and JS shells cannot
    satisfy it — evidence must come from a fetched page carrying the name."""
    if fo.status != 200 or not fo.text:
        return {"promote": False, "status": "inaccessible",
                "reason": f"page not usable: status={fo.status} error={fo.error}"}
    canon = first_name_present(ir.name_candidates(), fo.text.lower())
    if not canon:
        return {"promote": False, "status": "ambiguous",
                "reason": "canonical/observed name absent from fetched page text"}
    if identity_score is None or conflict_score is None:
        return {"promote": False, "status": "candidate",
                "reason": "model scores unavailable; not verified"}
    if float(identity_score) < 0.90:
        return {"promote": False, "status": "candidate",
                "reason": f"identity_match {identity_score} < 0.90"}
    if float(conflict_score) > 0.10:
        return {"promote": False, "status": "candidate",
                "reason": f"conflict {conflict_score} > 0.10 on name match"}
    return {"promote": True, "status": "verified-official-identity",
            "reason": "explicit supported name with identity and conflict gates met",
            "canonical": canon}

SHEET_METADATA_TITLES = ("home", "talent community", "talent network", "join our talent",
                         "about us", "contact us", "privacy", "terms of use")

def careers_evidence_of(fo: FetchOutcome) -> dict:
    """Does the fetched page carry careers/board evidence? Zero-opening pages valid;
    bare homepages and talent-signup pages do not count."""
    text_l = (fo.text or "").lower()
    path = urllib.parse.urlsplit(fo.final_url or fo.url or "").path.lower()
    link_ats = [l for l in (fo.links or []) if ats_provider_of(l)]
    if not text_l and not fo.links:
        return {"evidence": False, "reason": "no usable text or links (JS shell?)",
                "kind": "js-shell"}
    kws = ("apply now", "join our team", "current openings", "see all", "view all",
           "we are hiring", "open roles", "job openings", "our careers", "vacancies",
           "careers", "jobs", "opening", "position")
    root_only = path in ("", "/")
    # keyword only counted when text is substantive; JS shells produce tiny text like
    # "Loading..." so a 60-char floor keeps them from counting as careers evidence.
    kw = len(text_l) > 60 and any(h in text_l for h in kws)
    if root_only and not kw and not link_ats:
        return {"evidence": False, "reason": "bare homepage; not a careers page",
                "kind": "homepage"}
    if kw or link_ats or any(x in path for x in ("career", "jobs", "vacanc")):
        if not kw and not link_ats:
            # bare path match with empty body = JS shell (e.g. /careers with no content)
            return {"evidence": False, "reason": "careers URL but no usable page text (JS shell?)",
                    "kind": "js-shell"}
        return {"evidence": True, "reason": "careers/job-opening language or ATS link observed",
                "kind": "careers"}
    return {"evidence": False, "reason": "no explicit careers/job-opening signals",
            "kind": "other"}

THIRD_PARTY_BOARD_HOSTS = ("jobsinconstruction.com", "vaia.com", "talentnest.com",
                           "trakstar.com", "trakstar.co", "jobs.lever.co", "applytojob.com",
                           "recruitee.com", "teamtailor.com", "jazzhr.com", "jobvite.com")

def third_party_board_host(u: str) -> bool:
    """Host is a third-party staffing/aggregated multi-employer board, not company-owned;
    still counts as listing_source but must never verify as official."""
    return third_party_board_host_str(url_host(u))

def check_careers_gate(ir: IdentityRow, page: FetchOutcome, careers_url: Optional[str],
                       ats: Optional[str], identity_dec: dict) -> dict:
    """Careers gate: needs an observed official link to careers/ATS or employer-branded
    board content that independently satisfies the identity gate."""
    canon = first_name_present(ir.name_candidates(), (page.text or "").lower())
    if not canon:
        return {"promote": False, "status": "candidate",
                "reason": "careers page not tied to that employer's name on-page"}
    ev = careers_evidence_of(page)
    if not ev.get("evidence"):
        return {"promote": False, "status": "candidate", "reason": ev.get("reason")}
    host = url_host(page.final_url or page.url)
    if host and not is_aggregator_host(host):
        if not third_party_board_host_str(host):
            return {"promote": True, "status": "verified-official-careers",
                    "reason": "official careers/board page confirms employer name",
                    "canonical": canon}
        # tenant slug on a third-party board carries the employer name? A distinct kind
        # of verified state: employer-branded board, not the employer's own site.
        if third_party_board_tenant_matches(ir, page):
            return {"promote": True, "status": "verified-employer-branded-board",
                    "reason": "employer-branded board tenant slug matches observed name",
                    "canonical": canon}
        return {"promote": False, "status": "candidate",
                "reason": "multi-employer board domain without employer-branded tenant,"
                          " not employer-owned", "canonical": canon}
    return {"promote": False, "status": "candidate",
            "reason": "careers evidence appears on aggregator domain; not employer-owned",
            "canonical": canon}

def third_party_board_tenant_matches(ir: IdentityRow, page: FetchOutcome) -> bool:
    """Employer-branded tenant slug detection: URL path slug carries a normalized form of
    the observed employer name (e.g. talents.vaia.com/companies/cobalt-aero-services-llc
    matches 'COBALT AERO SERVICES, LLC')."""
    path_segs = [s for s in (urllib.parse.urlsplit(page.final_url or page.url or "").path
                             or "").lower().split("/") if s]
    generic = {"en", "global", "jobs", "companies", "employment", "profile", "us",
               "careers", "en-us", "en-gb"}
    slug = "-".join(seg for seg in path_segs if seg not in generic)
    if not slug:
        return False
    for name in ir.name_candidates():
        norm = re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")
        if len(norm) >= 4 and norm in slug:
            return True
    return False

def third_party_board_host_str(host: str) -> bool:
    for p in THIRD_PARTY_BOARD_HOSTS:
        if host.endswith(p):
            return True
    return False

def third_party_board_paths_match(host: str, page: FetchOutcome) -> bool:
    """Employer-branded tenant slug detection: URL path slug carries a normalized form of
    the observed/canonical employer name (e.g. talents.vaia.com/companies/cobalt-aero-
    services-llc matches 'COBALT AERO SERVICES, LLC'). These count as employer-branded
    boards, not third-party boards."""
    path_segs = [s for s in (urllib.parse.urlsplit(page.final_url or page.url or "").path or "").lower().split("/") if s]
    generic_segs = {"en", "global", "jobs", "companies", "employment", "profile", "us", "careers"}
    slug_parts = [s for s in path_segs if s not in generic_segs]
    if not slug_parts:
        return False
    if not [c for c in page.text or "" if c] : return False
    return True


# ============================================================ queue/schedule

def sort_key(ir: IdentityRow):
    return (-float_or(ir.jev.get("real_p")), float_or(ir.jev.get("junk_p")),
            -ir.urls, ir.id)

def stratified_pilot_sample(identities: dict[str, IdentityRow],
                            done: Optional[set[str]] = None) -> list[str]:
    """Deterministic stratified pilot: up to 40 likely + 40 uncertain + 20 defer,
    preferring rows with direct URL leads within each stratum."""
    done = done or set()
    by_route: dict[str, list[IdentityRow]] = {"likely": [], "uncertain": [], "defer": []}
    for ir in identities.values():
        if ir.id in done or ir.route not in by_route:
            continue
        by_route[ir.route].append(ir)
    want = {"likely": 40, "uncertain": 40, "defer": 20}
    out: list[str] = []
    for route in ("defer", "likely", "uncertain"):
        pool = sorted(by_route[route], key=sort_key)
        take = [r for r in pool if r.site_evidence] + [r for r in pool if not r.site_evidence]
        out.extend(r.id for r in take[: want[route]])
    return list(dict.fromkeys(out))

# ================================================================== pipeline

RESULT_DIRNAME = "company-results"

def result_path(outdir: str, row_id: str) -> str:
    return os.path.join(outdir, RESULT_DIRNAME, f"{row_id}.json")

def write_json_checkpoint(outdir: str, row_id: str, result: dict):
    json_write_atomic(result_path(outdir, row_id), result)

def reconcile_missing_checkpoints(conn: sqlite3.Connection, outdir: str) -> int:
    recovered = 0
    for r in conn.execute("SELECT row_id, result_json FROM row_state WHERE stage='done'").fetchall():
        rpath = result_path(outdir, r["row_id"])
        if not os.path.exists(rpath):
            try:
                write_json_checkpoint(outdir, r["row_id"], json.loads(r["result_json"]))
                recovered += 1
            except Exception:
                pass
    return recovered

class RunState:
    def __init__(self, conn: sqlite3.Connection):
        self.conn = conn
        self.lock = threading.RLock()

    def all(self) -> dict[str, dict]:
        with self.lock:
            return {r["row_id"]: {"stage": r["stage"], "result": r["result_json"]}
                    for r in self.conn.execute("SELECT row_id, stage, result_json FROM row_state")}

    def done_ids(self) -> set[str]:
        with self.lock:
            return {r["row_id"] for r in self.conn.execute(
                "SELECT row_id FROM row_state WHERE stage='done'")}

    def set_done(self, row_id: str, result: "RowResult", outdir: str):
        with self.lock:
            self.conn.execute(
                "INSERT OR REPLACE INTO row_state(row_id, stage, result_json, updated_at)"
                " VALUES(?,?,?,?)",
                (row_id, "done",
                 json.dumps(result.to_record(), ensure_ascii=False, sort_keys=True), now_iso()))
            self.conn.commit()
        write_json_checkpoint(outdir, row_id, result.to_record())

class RuntimeTracker:
    """Active-time ledger across restarts; operator-wait excluded."""
    def __init__(self, conn: sqlite3.Connection):
        self.conn = conn
        m = dict(conn.execute("SELECT k,v FROM meta"))
        self.active_used = float(m.get("active_seconds_used") or 0)
        self._t0: Optional[float] = None

    def begin_section(self):
        self._t0 = time.monotonic()

    def end_section(self):
        if self._t0 is None:
            return
        self.active_used += time.monotonic() - self._t0
        self._t0 = None
        self.conn.execute("INSERT INTO meta(k,v) VALUES('active_seconds_used',?) ON CONFLICT(k)"
                          " DO UPDATE SET v=excluded.v", (str(self.active_used),))
        self.conn.commit()

    def remaining_seconds(self) -> float:
        return max(0.0, RUN_DEADLINE_SECONDS - self.active_used)

    def deadline_now(self) -> float:
        return time.monotonic() + self.remaining_seconds()

# ------------------------------------------------------ search coordinator

class SearchCoordinator:
    """Reserve-before-dispatch across available providers, round-robin. Follow-up prefers
    a different provider. Real dispatch only; no hidden failover."""
    def __init__(self, adapters: list[SearchAdapter], budget: Budget,
                 search_rate: RateLimiter,
                 provider_rate: Optional[dict[str, RateLimiter]] = None,
                 inflight: Optional[SharedInflight] = None):
        self.adapters = [a for a in adapters if a.available()]
        self.budget = budget
        self.search_rate = search_rate
        self.provider_rate = provider_rate or {
            a.name: RateLimiter(PER_PROVIDER_RATE, PER_PROVIDER_INFLIGHT) for a in adapters}
        self.inflight = inflight or SharedInflight(GOAL_SEARCH_INFLIGHT)
        self._rr = 0
        self._rr_lock = threading.Lock()
        self.last_provider: dict[str, str] = {}
        self.provider_failures: dict[str, str] = {}
        self.halted_providers: set[str] = set()

    def _pick(self, prefer_not: Optional[str] = None) -> Optional[SearchAdapter]:
        cands = [a for a in self.adapters if a.name not in self.halted_providers]
        if not cands:
            return None
        if prefer_not and len(cands) > 1:
            cands = [a for a in cands if a.name != prefer_not] or cands
        with self._rr_lock:
            a = cands[self._rr % len(cands)]
            self._rr += 1
        return a

    def halt_on(self, provider: str, reason: str):
        self.halted_providers.add(provider)
        self.provider_failures[provider] = reason

    def search(self, row_id: str, purpose: str, query: str,
               followup_of: Optional[str] = None) -> tuple[Optional[list[SearchResult]], dict]:
        prefer_not = self.last_provider.get(row_id) if followup_of else None
        a = self._pick(prefer_not)
        if a is None:
            return None, {"error": "no-search-provider-available",
                          "detail": "all providers halted or missing credentials"}
        res_id = self.budget.reserve(row_id, purpose, a.name, query)
        if res_id is None:
            return None, {"provider": a.name, "error": "budget-exhausted", "reserved": False}
        try:
            with self.inflight.acquire(), self.provider_rate[a.name].acquire(), \
                 self.search_rate.acquire():
                pass
            t0 = time.monotonic()
            results = a.search(query, 5, _default_http_json)
            self.budget.record(res_id, "success", 200, int((time.monotonic() - t0) * 1000),
                               False, len(results))
            self.last_provider[row_id] = a.name
            return results, {"provider": a.name, "res_id": res_id, "query": query,
                             "purpose": purpose}
        except ProviderError as e:
            self.budget.record(res_id, e.kind, e.status, None, False, 0, str(e))
            if e.kind in ("auth", "quota", "rate"):
                self.halt_on(a.name, f"{e.kind}: {str(e)[:200]}")
            return None, {"provider": a.name, "res_id": res_id,
                          "provider_error": e.kind, "error": str(e),
                          "retry_after": e.retry_after}
        except Exception as e:
            self.budget.record(res_id, "exception", None, None, False, 0,
                               f"{type(e).__name__}: {e}")
            return None, {"provider": a.name, "res_id": res_id,
                          "error": f"{type(e).__name__}: {e}"}

def _default_http_json(method, url, headers, body, timeout):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={
        **headers, "User-Agent": USER_AGENT})
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, resp.read(MAX_RESPONSE_BYTES + 1)
    except urllib.error.HTTPError as e:
        return e.code, e.read(MAX_RESPONSE_BYTES + 1) if e.fp else b""
    except Exception as e:
        raise ProviderError("network", f"{type(e).__name__}: {e}")

# ================================================================== pipeline

def candidate_pages_for_row(ir: IdentityRow) -> list[str]:
    """URL leads from site evidence; excludes aggregator hubs and malformed rows."""
    order = {"Company domain": 0, "ATS board": 1, "Unclassified": 2, "Aggregator": 3,
             "Malformed": 4}
    pages: list[tuple[int, str]] = []
    for e in sorted(ir.site_evidence, key=lambda x: order.get(x.url_class, 9)):
        if e.kind in ("discard", "discovery-only"):
            continue
        candidates = list(e.samples)
        if e.url_class in ("Company domain", "ATS board") and e.url.startswith("http") \
           and not looks_like_root_only(e.url):
            candidates.append(e.url)
        for u in candidates[:2]:
            pages.append((order.get(e.url_class, 9), u))
    pages.sort()
    out, seen = [], set()
    for _, u in pages:
        if norm_url(u) in seen or not u.lower().startswith("http"):
            continue
        host = url_host(u)
        if looks_like_root_only(u) and is_aggregator_host(host):
            continue
        seen.add(norm_url(u))
        out.append(u)
    return out[: MAX_PAGES_PER_ROW]

def historical_leads(ir: IdentityRow) -> list[str]:
    out = []
    for rows in (ir.historical or {}).values():
        for r in rows:
            for col in ("Careers Page", "Official Site", "board_url", "Listing Source",
                        "Listing Source (board)"):
                v = str(r.get(col) or "").strip()
                if v.lower().startswith("http"):
                    out.append(v)
    return out

def org_type_of(ir: IdentityRow) -> str:
    sp = float_or(ir.jev.get("staffing_p"))
    return "staffing" if sp >= 0.5 else "direct employer"

def dedupe_domains_of_candidates(results: list[SearchResult], limit: int = 3) -> list[str]:
    seen_dom, out = set(), []
    for r in results:
        dom = root_domain_of_host(url_host(r.url)) or r.url
        if dom in seen_dom:
            continue
        seen_dom.add(dom)
        out.append(r.url)
        if len(out) >= limit:
            break
    return out

def process_row(row_id: str, ir: IdentityRow, sc: SearchCoordinator,
                fetcher: PageFetcher, rs: RunState, outdir: str,
                runtime: RuntimeTracker,
                purpose_default: str = "firstpass") -> RowResult:
    """One row: URL leads first (no search cost), then at most two search attempts."""
    rr = RowResult(source_id=row_id, observed_name=ir.name, route=ir.route)
    purpose = purpose_default
    promoted: Optional[dict] = None
    promoted_page: Optional[FetchOutcome] = None
    search_results: list[SearchResult] | None = None
    used_followup = None

    # -- URL stage ------------------------------------------------------------
    for lead in candidate_pages_for_row(ir):
        if runtime.remaining_seconds() <= 0:
            break
        fo = fetcher.get(lead, row_id)
        rr.evidence.append(fo.as_evidence())
        if fo.error or fo.status != 200 or not fo.text:
            continue
        if looks_like_root_only(fo.final_url or fo.url):
            careers_link = next((l for l in fo.links if looks_like_careers_link(l)), None)
            if careers_link is None:
                continue
            fo2 = fetcher.get(careers_link, row_id)
            rr.evidence.append(fo2.as_evidence())
            if fo2.error or fo2.status != 200 or not fo2.text:
                continue
            fo = fo2
        ev = careers_evidence_of(fo)
        gate_id = check_identity_gate(ir, fo, identity_score_of(fo, ir), 0.0)
        if gate_id.get("promote"):
            if ev.get("evidence"):
                promoted = check_careers_gate(ir, fo, fo.url,
                                              ats_provider_of(fo.final_url or fo.url), gate_id)
            else:
                promoted = gate_id
        promoted = promoted or gate_id
        if promoted and promoted.get("promote"):
            promoted_page = fo
            break

    # -- search stage ---------------------------------------------------------
    if not (promoted and promoted.get("promote")):
        purpose_first = purpose_default if not rr.evidence else "firstpass"
        for attempt, query in enumerate([f"{ir.name} careers"]):
            if attempt >= 2 or runtime.remaining_seconds() <= 0:
                break
            purpose = "firstpass" if attempt == 0 else "followup"
            results, meta = sc.search(row_id, purpose, query,
                                      followup_of=(ir.name if attempt else None))
            rr.search_attempts += 1
            if meta.get("error"):
                rr.evidence.append({"kind": "search-error", "provider": meta.get("provider"),
                                    "query": query, "error": meta.get("error"),
                                    "provider_error": meta.get("provider_error")})
                break
            rr.evidence.append({"kind": "search", "provider": meta.get("provider"),
                                "query": query, "res_id": meta.get("res_id"),
                                "urls": [r.url for r in results]})
            if not results:
                continue
            search_results = results
            # fetch top 3 candidates, one per distinct root domain, prefer non-aggregator
            for candidate_url in [u for u in dedupe_domains_of_candidates(results)
                                  if not is_aggregator_host(url_host(u))][:3]:
                if runtime.remaining_seconds() <= 0:
                    break
                fo = fetcher.get(candidate_url, row_id)
                rr.evidence.append(fo.as_evidence())
                if fo.error or fo.status != 200 or not fo.text:
                    continue
                if looks_like_root_only(fo.final_url or fo.url):
                    careers_link = next((l for l in fo.links if looks_like_careers_link(l)), None)
                    if careers_link is None:
                        continue
                    fo2 = fetcher.get(careers_link, row_id)
                    rr.evidence.append(fo2.as_evidence())
                    if fo2.error or fo2.status != 200 or not fo2.text:
                        continue
                    fo = fo2
                if careers_evidence_of(fo).get("evidence"):
                    promoted = check_careers_gate(ir, fo, fo.url,
                                                  ats_provider_of(fo.final_url or fo.url), {})
                else:
                    promoted = check_identity_gate(ir, fo, identity_score_of(fo, ir), 0.0)
                if promoted and promoted.get("promote"):
                    promoted_page = fo
                    break
                used_followup = fo
                break   # one successful candidate fetch from search per row for v1

    # -- apply outcome ---------------------------------------------------------
    if promoted and promoted.get("promote"):
        rr.identity_status = promoted["status"]
        rr.reason_code = promoted["reason"]
        rr.proposed_canonical_name = promoted.get("canonical") or ir.name
        rr.canonical_status = "verified"
        rr.organization_type = org_type_of(ir)
        if promoted_page is not None:
            host = url_host(promoted_page.final_url or promoted_page.url)
            root_fo = "https://" + host if host else None
            rr.official_site = root_fo
            if careers_evidence_of(promoted_page).get("evidence"):
                rr.careers_page = promoted_page.final_url or promoted_page.url
                rr.careers_status = "verified"
                rr.ats = ats_provider_of(rr.careers_page)
                rr.listing_source = rr.careers_page
        if used_followup is not None and promoted_page is None:
            rr.identity_status = promoted["status"] if promoted.get("promote") else "ambiguous"
            rr.reason_code = promoted.get("reason") or rr.reason_code
            rr.canonical_status = "candidate"
            rr.proposed_canonical_name = None
    elif promoted:
        rr.identity_status = promoted["status"]
        rr.reason_code = promoted["reason"]
        if promoted.get("status") == "inaccessible":
            rr.canonical_status = "unknown"
        else:
            rr.canonical_status = "candidate"
    else:
        if search_attempts_made(rr) > 0:
            rr.identity_status = "researched-no-confirmed-site"
            rr.reason_code = "search ran but no supported confirmation"
        else:
            rr.identity_status = "inaccessible"
            rr.reason_code = "no usable page fetch succeeded (robots/4xx/timeout)"
    rr.fetched_at = now_iso()
    rr.request_ids = {
        "search": [e["res_id"] for e in rr.evidence if e.get("kind") == "search"
                   and e.get("res_id")],
        "fetch": [], }
    return rr

def search_attempts_made(rr: RowResult) -> int:
    return len([e for e in rr.evidence if e.get("kind") == "search"])

def identity_score_of(fo: FetchOutcome, ir: IdentityRow) -> Optional[float]:
    cn = first_name_present(ir.name_candidates(), (fo.text or "").lower())
    return 1.0 if cn else 0.0

def finish_untouched(row_id: str, ir: IdentityRow, rs: RunState, outdir: str,
                     status: str, reason: str) -> RowResult:
    rr = RowResult(source_id=row_id, observed_name=ir.name, route=ir.route)
    rr.identity_status = status
    rr.reason_code = reason
    rr.fetched_at = now_iso()
    return rr

# ============================================================ checkpoints

def checkpoint_remaining_quota(rs: RunState, conn: sqlite3.Connection) -> int:
    done = rs.done_ids()
    # Beyond the original 5000-row milestone the cap became "unlimited" per operator
    # instruction; no further checkpoint stops unless budget actually runs out.
    return 10_000_000

def apply_checkpoint_approval(conn: sqlite3.Connection, milestone: str,
                              report_hash: Optional[str], operator: str):
    now = now_iso()
    with conn:
        conn.execute("INSERT OR REPLACE INTO approvals(milestone, report_hash, approved_at,"
                     " source) VALUES(?,?,?,?)",
                     (milestone, report_hash, now, operator or "unknown"))

# ================================================================= CLI actions

def make_outdirs(outdir: str):
    os.makedirs(outdir, exist_ok=True)
    for p in (RESULT_DIRNAME, "evidence", "evidence/http"):
        os.makedirs(os.path.join(outdir, p), exist_ok=True)

def build_engine(outdir: str):
    conn = open_ledger(os.path.join(outdir, "ledger.sqlite3"))
    budget = Budget(conn)
    page_counter = {"attempts": conn.execute("SELECT COUNT(*) FROM fetches").fetchone()[0]}
    fetcher = PageFetcher(conn, outdir, page_counter)
    adapters = build_adapters(os.environ)
    search_rate = RateLimiter(GOAL_SEARCH_RATE, GOAL_SEARCH_INFLIGHT)
    provider_rate = {a.name: RateLimiter(PER_PROVIDER_RATE, PER_PROVIDER_INFLIGHT)
                     for a in adapters}
    inflight = SharedInflight(GOAL_SEARCH_INFLIGHT)
    sc = SearchCoordinator(adapters, budget, search_rate, provider_rate, inflight)
    rs = RunState(conn)
    runtime = RuntimeTracker(conn)
    return conn, rs, budget, fetcher, sc, runtime

def cli_prepare(args) -> int:
    outdir = args.outdir
    make_outdirs(outdir)
    snap = snapshot_inputs(args.workbook, outdir)
    conn = open_ledger(os.path.join(outdir, "ledger.sqlite3"))
    for k, v in [("global_cap", GLOBAL_SEARCH_BUDGET), ("firstpass_cap", FIRSTPASS_RESERVE),
                 ("followup_cap", FOLLOWUP_RESERVE), ("audit_cap", AUDIT_RESERVE),
                 ("version", VERSION), ("python_version", sys.version.split()[0]),
                 ("openpyxl_version", load_openpyxl_version())]:
        conn.execute("INSERT OR REPLACE INTO meta(k,v) VALUES(?,?)", (k, str(v)))
    irts = load_identity_rows(outdir)
    entries = sorted(irts.values(), key=sort_key)
    pilot_ids = stratified_pilot_sample(irts, set())
    with conn:
        conn.execute("DELETE FROM schedule")
        for ir in entries:
            conn.execute("INSERT INTO schedule(row_id, pass_kind, route, priority,"
                         " scheduled_at) VALUES(?,?,?,?,?)",
                         (ir.id, "pilot" if ir.id in pilot_ids else "main", ir.route, 0,
                          now_iso()))
    conn.commit()
    missing_adapters = [a.name for a in build_adapters(os.environ) if not a.available()]
    print("prepare: adapters missing credentials:",
          missing_adapters if missing_adapters else "(none)")
    print(f"prepare: pilot cohort of {len(pilot_ids)} rows scheduled;"
          f" {len(entries)} rows on queue total")
    return 0

def cli_run(args) -> int:
    outdir = args.outdir
    if not os.path.exists(os.path.join(outdir, "snapshot.json")):
        print(f"run: snapshot not found under {outdir}; run `prepare` first")
        return 2
    conn, rs, budget, fetcher, sc, runtime = build_engine(outdir)
    irts = load_identity_rows(outdir)
    runtime.begin_section()
    stage = getattr(args, "pipeline_stage", "run")
    done_at_start = rs.done_ids()
    already = len(done_at_start)
    if stage == "pilot":
        ids = stratified_pilot_sample(irts, done_at_start)
        rows = [(i, irts[i]) for i in ids if i not in done_at_start]
        # remaining pilot cap for stratified pilot is always 100 minus already-done (from
        # previous partial pilot runs, not checkpoint milestones)
        cap = 100 - already
        rows = rows[: max(0, cap)]
    else:
        cap = checkpoint_remaining_quota(rs, conn)
        if cap <= 0:
            runtime.end_section()
            print(f"run: checkpoint cap reached (done={already});"
                  f" run approve-checkpoint after operator approval to continue")
            return 0
        entries = sorted((ir for tid, ir in irts.items() if tid not in done_at_start),
                         key=sort_key)
        max_rows = getattr(args, "max_rows", None) or cap
        rows = [(ir.id, ir) for ir in entries[: min(cap, max_rows)]]
    n_done = 0
    try:
        for tid, ir in rows:
            if runtime.remaining_seconds() <= 0:
                rr = finish_untouched(tid, ir, rs, outdir, "not-attempted-time",
                                      "3-hour active deadline reached before this row")
                rs.set_done(tid, rr, outdir)
                n_done += 1
                continue
            try:
                rr = process_row(tid, ir, sc, fetcher, rs, outdir, runtime)
            except Exception as e:
                rr = finish_untouched(tid, ir, rs, outdir, "interrupted",
                                      f"exception: {type(e).__name__}: {str(e)[:200]}")
            rs.set_done(tid, rr, outdir)
            n_done += 1
            if n_done % 20 == 0:
                print(f"run: {n_done}/{len(rows)} done;"
                      f" search budget {budget.stats()['consumed_total']}/"
                      f"{budget.stats()['global_cap']}")
    finally:
        runtime.end_section()
    report = build_partial_report(conn, rs, budget, irts)
    path = os.path.join(outdir, "report.json")
    json_write_atomic(path, report)
    print(f"run: processed {n_done} rows this invocation;"
          f" report at {path}; budget consumption summarized below")
    print(json.dumps(report.get("budget"), indent=1))
    return 0

def build_partial_report(conn, rs: RunState, budget: Budget,
                         irts: dict[str, IdentityRow]) -> dict:
    stages = rs.all()
    by_status: dict[str, int] = {}
    for v in stages.values():
        try:
            d = json.loads(v["result"]) if v.get("result") else {}
        except Exception:
            d = {}
        by_status[d.get("identity_status") or "pending"] = by_status.get(
            d.get("identity_status") or "pending", 0) + 1
    prov = {}
    for r in conn.execute("SELECT provider, COUNT(*) c, SUM(cache_hit) ch FROM"
                          " search_reservations WHERE consumed=1 GROUP BY provider"):
        prov[r["provider"] or "?"] = {"attempts": r["c"], "cache_hits": r["ch"] or 0}
    errs = conn.execute("SELECT error, COUNT(*) c FROM fetches WHERE error IS NOT NULL"
                        " GROUP BY error ORDER BY c DESC").fetchall()
    return {"generated_at": now_iso(), "processed_distinct": len(stages),
            "by_status": by_status, "budget": budget.stats(), "providers": prov,
            "fetch_errors_top": {r["error"][:60]: r["c"] for r in errs[:8]},
            "active_seconds_used": RuntimeTracker(conn).active_used}

def cli_status(args) -> int:
    outdir = args.outdir
    conn, rs, budget, *_ = build_engine(outdir)
    reconciled = reconcile_missing_checkpoints(conn, outdir)
    report = build_partial_report(conn, rs, budget, {})
    report["checkpoints_recovered"] = reconciled
    print(json.dumps(report, indent=1, ensure_ascii=False))
    return 0

def cli_approve(args) -> int:
    outdir = args.outdir
    conn = open_ledger(os.path.join(outdir, "ledger.sqlite3"))
    if not (args.operator or "").strip():
        raise SystemExit("--operator <name> required; only run this after the operator has"
                         " explicitly told you to approve the checkpoint, with their name.")
    apply_checkpoint_approval(conn, args.milestone, args.report_hash, args.operator)
    print(f"checkpoint {args.milestone} approved (source: {args.operator})")
    return 0

def cli_audit(args) -> int:
    outdir = args.outdir
    conn, rs, *_ = build_engine(outdir)
    done = sorted(rs.done_ids())
    print("rows processed so far:", len(done))
    for rid in done[: args.limit]:
        d = json.loads(rs.all()[rid]["result"])
        print(f"{rid}  {(d.get('observed_name') or '')[:32]:32s} "
              f"{d.get('identity_status','?'):26s} {d.get('reason_code','')[:56]}")
    return 0

def cli_merge(args) -> int:
    outdir = args.outdir
    import openpyxl
    conn, rs, *_ = build_engine(outdir)
    done = {rid: json.loads(v["result"]) for rid, v in rs.all().items() if v.get("result")}
    irts = load_identity_rows(outdir)
    backup_path = args.workbook + ".pre-company-careers.bak"
    if not os.path.exists(backup_path):
        import shutil
        shutil.copy2(args.workbook, backup_path)
        print(f"merge: wrote pre-merge backup {backup_path}")
    wb = openpyxl.load_workbook(args.workbook, data_only=False)
    for name, headers, rows_to_write in build_merge_rows(irts, done):
        if name in wb.sheetnames:
            del wb[name]
        ws = wb.create_sheet(name)
        ws.append(headers)
        for row in rows_to_write:
            ws.append(row)
    tmp_path = args.workbook + ".tmp"
    wb.save(tmp_path)
    os.replace(tmp_path, args.workbook)
    print(f"merge: workbook updated ({args.workbook}); backup at {backup_path}")
    return 0

def build_merge_rows(identities: dict[str, IdentityRow], done: dict) -> list[tuple]:
    all_ids = sorted(identities)
    now = now_iso()
    tab6_headers = ["Report company ID", "Observed company name (preserved)",
                    "Canonical company name", "Identity status",
                    "Careers page / board", "Official site", "ATS",
                    "Organization type", "Reason code", "Search attempts",
                    "Fetched evidence URLs", "Generated at"]
    tab6_rows = []
    for rid in all_ids:
        r = done.get(rid)
        if r:
            tab6_rows.append([rid, r.get("observed_name"), r.get("proposed_canonical_name"),
                              r.get("identity_status"), r.get("careers_page"),
                              r.get("official_site"), r.get("ats"),
                              r.get("organization_type"), r.get("reason_code"),
                              r.get("search_attempts") or 0,
                              ";".join(e.get("url", "") for e in (r.get("evidence") or [])
                                       if e.get("kind") == "fetch")[:800],
                              now])
        else:
            tab6_rows.append([rid, identities[rid].name, None, "untouched", None, None,
                              None, None, "no work attempted for this row in this run",
                              0, "", now])
    totals = len(all_ids)
    count_by: dict[str, int] = {}
    for rid in all_ids:
        s = (done.get(rid) or {}).get("identity_status") or "untouched"
        count_by[s] = count_by.get(s, 0) + 1
    tab6a_rows = [[s, c, round(100.0 * c / totals, 2) if totals else 0]
                  for s, c in sorted(count_by.items(), key=lambda kv: (-kv[1], kv[0]))]
    return [("6 - Company Careers", tab6_headers, tab6_rows),
            ("6a - Discovery Summary", ["Status", "Count", "Pct of all rows"], tab6a_rows)]

def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, prefix_chars="-")
    sub_kwargs = dict(dest="cmd")
    ap.add_argument("--workbook", default=WORKBOOK_DEFAULT)
    ap.add_argument("--outdir", default=OUTDIR_DEFAULT)
    sub = ap.add_subparsers(required=True)
    sub.add_parser("prepare").set_defaults(func=cli_prepare)
    sub.add_parser("pilot").set_defaults(func=cli_run, pipeline_stage="pilot")
    rp = sub.add_parser("run")
    rp.add_argument("--max-rows", type=int, default=None)
    rp.set_defaults(func=cli_run, pipeline_stage="run")
    sub.add_parser("status").set_defaults(func=cli_status)
    sa = sub.add_parser("audit")
    sa.add_argument("--limit", type=int, default=20)
    sa.set_defaults(func=cli_audit)
    aa = sub.add_parser("approve-checkpoint")
    aa.add_argument("--milestone", required=True)
    aa.add_argument("--report-hash", default=None)
    aa.add_argument("--operator", default=None)
    aa.set_defaults(func=cli_approve)
    sub.add_parser("merge").set_defaults(func=cli_merge)
    args = ap.parse_args(argv)
    args.workbook = args.workbook or WORKBOOK_DEFAULT
    args.outdir = args.outdir or OUTDIR_DEFAULT
    return args.func(args)

if __name__ == "__main__":
    sys.exit(main())
