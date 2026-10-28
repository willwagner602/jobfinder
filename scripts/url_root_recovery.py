#!/usr/bin/env python3
"""url_root_recovery.py — recover careers-site roots for low-quality career URLs.

Reads the live jobs DB read-only, classifies problematic careers_page rows
(job-detail URLs, bare shared ATS hosts, non-employer sites), derives candidate
board roots, verifies them with polite HTTP fetches, and appends resumable
JSONL results under artifacts/url-root-recovery/. Never writes to the DB.
"""
from __future__ import annotations

import argparse
import json
import re
import sys
import threading
import time
import urllib.parse
import urllib.request
import uuid
from concurrent.futures import ThreadPoolExecutor

import sqlite3

UA = ("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/126.0 Safari/537.36")
TIMEOUT = 15
CONCURRENCY = 6
HOST_PAUSE = 0.4

DETAIL_RE = re.compile(
    r'(/Details/|/details/\d|/job/|/jobs/\d{4,}|/position/|/posting/|'
    r'/opportunit(y|ies)/\d|/requisition|/opening/|/careers/job|/jobs/view|/en/job)', re.I)

NOT_EMPLOYER_HOSTS = {
    'en.wikipedia.org', 'www.wikipedia.org', 'www.google.com',
    'www.linkedin.com', 'www.indeed.com', 'www.glassdoor.com',
    'www.greatplacetowork.com', 'www.ziprecruiter.com',
}

MULTI_TENANT_HOSTS = {
    'jobs.ashbyhq.com', 'recruiting.paylocity.com', 'apply.workable.com',
    'jobs.gem.com', 'jobs.workable.com', 'jobs.lever.co',
    'boards.greenhouse.io', 'job-boards.greenhouse.io',
    'jobs.jobvite.com', 'jobs.smartrecruiters.com',
}

SUFFIX_TOKENS = {'inc', 'llc', 'ltd', 'corp', 'corporation', 'company', 'co',
                 'the', 'and', 'llp', 'lp', 'plc', 'group', 'holdings',
                 'technologies', 'technology'}

host_locks: dict[str, threading.Lock] = {}
locks_guard = threading.Lock()


def host_of(url: str) -> str:
    try:
        return urllib.parse.urlsplit(url).netloc.lower()
    except Exception:
        return ''


def name_tokens(name: str) -> list[str]:
    toks = re.findall(r'[a-z0-9]+', name.casefold())
    return [t for t in toks if t not in SUFFIX_TOKENS and len(t) > 1]


def slugify(name: str) -> str:
    toks = name_tokens(name)
    return '-'.join(toks) if toks else ''


def classify(url: str, host_path_counts: dict[str, int]) -> str:
    try:
        p = urllib.parse.urlsplit(url)
    except Exception:
        return 'unparseable'
    host, path = p.netloc.lower(), p.path
    if host in NOT_EMPLOYER_HOSTS:
        return 'non-employer'
    if DETAIL_RE.search(path):
        return 'job-detail'
    if path in ('', '/'):
        if host in MULTI_TENANT_HOSTS or host_path_counts.get(host, 0) > 1:
            return 'bare-shared-host'
        return 'bare-company-host'  # acceptable, kept
    return 'ok'


def fetch(url: str, method: str = 'GET', data: bytes | None = None,
          headers: dict | None = None) -> tuple[int, str]:
    h = {'User-Agent': UA, 'Accept-Language': 'en'}
    if headers:
        h.update(headers)
    req = urllib.request.Request(url, data=data, headers=h, method=method)
    with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
        charset = resp.headers.get_content_charset() or 'utf-8'
        return resp.status, resp.read(400_000).decode(charset, 'ignore')


def polite_fetch(url: str, **kw) -> tuple[int, str]:
    host = host_of(url)
    with locks_guard:
        lock = host_locks.setdefault(host, threading.Lock())
    with lock:
        time.sleep(HOST_PAUSE)
        for attempt in (1, 2):
            try:
                return fetch(url, **kw)
            except urllib.error.HTTPError as e:
                if e.code in (429, 503) and attempt == 1:
                    time.sleep(2.0)
                    continue
                return e.code, ''
            except Exception:
                if attempt == 1:
                    time.sleep(1.0)
                    continue
                return 0, ''


def name_in(text: str, tokens: list[str]) -> bool:
    if not tokens:
        return False
    low = text.casefold()
    hits = sum(1 for t in set(tokens) if t in low)
    return hits >= min(2, len(set(tokens)))


def candidates_for(cat: str, url: str, name: str) -> list[tuple[str, str]]:
    """Return list of (kind, candidate) probes. kind: 'url' or 'ashby-api'."""
    p = urllib.parse.urlsplit(url)
    host, path = p.netloc.lower(), p.path.rstrip('/')
    out: list[tuple[str, str]] = []
    slugs = []
    base = slugify(name)
    if base:
        slugs.append(base)
        short = slugify(' '.join(name_tokens(name)[:3])) if len(name_tokens(name)) > 3 else base
        if short and short != base:
            slugs.append(short)
    if cat == 'job-detail':
        segs = [s for s in path.split('/') if s]
        if host in MULTI_TENANT_HOSTS and segs:
            out.append(('url', f'https://{host}/{segs[0]}'))
        out.append(('url', f'https://{host}/'))
    elif cat == 'bare-shared-host':
        if host == 'jobs.ashbyhq.com':
            for s in slugs:
                out.append(('ashby-api', f'https://api.ashbyhq.com/postingApi/jobBoard/{s}'))
                out.append(('url', f'https://jobs.ashbyhq.com/{s}'))
        elif host in ('apply.workable.com', 'jobs.workable.com'):
            for s in slugs:
                out.append(('url', f'https://apply.workable.com/{s}'))
        elif host == 'jobs.gem.com':
            for s in slugs:
                out.append(('url', f'https://jobs.gem.com/{s}'))
        else:
            for s in slugs:
                out.append(('url', f'https://{host}/{s}'))
    return out[:6]


def probe(kind: str, target: str, tokens: list[str]) -> dict:
    if kind == 'ashby-api':
        status, body = polite_fetch(target)
        if status == 200 and body:
            try:
                data = json.loads(body)
                blob = json.dumps(data)[:4000]
                if name_in(blob, tokens):
                    org = data.get('organizationName') or data.get('name') or ''
                    return {'ok': True, 'tier': 'name-match', 'url':
                            target.replace('https://api.ashbyhq.com/postingApi/jobBoard/',
                                           'https://jobs.ashbyhq.com/'),
                            'org': org, 'status': status}
            except Exception:
                pass
        return {'ok': False, 'status': status}
    status, body = polite_fetch(target)
    if status != 200 or not body:
        return {'ok': False, 'status': status}
    title = re.search(r'<title[^>]*>(.*?)</title>', body, re.I | re.S)
    text = (title.group(1) if title else '') + ' ' + body[:4000]
    if name_in(text, tokens):
        return {'ok': True, 'tier': 'name-match', 'url': target, 'status': status}
    return {'ok': True, 'tier': 'alive-only', 'url': target, 'status': status}


def recover(row: dict, host_path_counts: dict) -> dict:
    cat = classify(row['careers_page'], host_path_counts)
    result = {'source_id': row['source_id'], 'company': row['company'],
              'original_url': row['careers_page'], 'category': cat,
              'recovered_url': None, 'tier': None, 'candidates': [],
              'recovered_at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())}
    if cat in ('ok', 'bare-company-host'):
        result['tier'] = 'no-recovery-needed'
        return result
    if cat == 'non-employer':
        result['tier'] = 'not-derivable'
        return result
    tokens = name_tokens(row['company'])
    best = None
    for kind, cand in candidates_for(cat, row['careers_page'], row['company']):
        r = probe(kind, cand, tokens)
        result['candidates'].append({'kind': kind, 'target': cand, **r})
        if r.get('ok') and r.get('tier') == 'name-match':
            best = r
            break
        if r.get('ok') and best is None:
            best = r
    if best and best.get('tier') == 'name-match':
        result['recovered_url'] = best['url']
        result['tier'] = 'verified'
    elif best:
        result['recovered_url'] = None
        result['tier'] = 'unverified-candidate'
        result['candidate_url'] = best.get('url')
    else:
        result['tier'] = 'not-derivable'
    return result


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--db', default='data/jobs.sqlite3')
    ap.add_argument('--out', default='artifacts/url-root-recovery')
    ap.add_argument('--limit', type=int, default=0)
    ap.add_argument('--only', choices=['job-detail', 'bare-shared-host', 'non-employer'], default='')
    args = ap.parse_args()

    conn = sqlite3.connect(f'file:{args.db}?mode=ro', uri=True)
    rows = conn.execute(
        """SELECT source_id, COALESCE(NULLIF(canonical_name,''),observed_name),
                  careers_page, identity_status
           FROM company_careers
           WHERE careers_page IS NOT NULL AND trim(careers_page)<>''"""
    ).fetchall()
    conn.close()

    host_path_counts: dict[str, int] = {}
    for _sid, _name, url, _st in rows:
        try:
            h = urllib.parse.urlsplit(url).netloc.lower()
            if urllib.parse.urlsplit(url).path not in ('', '/'):
                host_path_counts[h] = host_path_counts.get(h, 0) + 1
        except Exception:
            pass

    problems = []
    for sid, name, url, status in rows:
        cat = classify(url, host_path_counts)
        if cat in ('ok', 'bare-company-host'):
            continue
        if args.only and cat != args.only:
            continue
        problems.append({'source_id': sid, 'company': name,
                         'careers_page': url, 'identity_status': status, 'category': cat})
    if args.limit:
        problems = problems[:args.limit]

    import os
    os.makedirs(args.out, exist_ok=True)
    results_path = os.path.join(args.out, 'results.jsonl')
    done: set[str] = set()
    if os.path.exists(results_path):
        with open(results_path) as f:
            for line in f:
                try:
                    done.add(json.loads(line)['source_id'])
                except Exception:
                    pass
    pending = [r for r in problems if r['source_id'] not in done]
    print(f'problems={len(problems)} already_done={len(done)} pending={len(pending)}', flush=True)

    lock = threading.Lock()
    with open(results_path, 'a', buffering=1) as out, \
         ThreadPoolExecutor(max_workers=CONCURRENCY) as pool:
        counted = {'n': 0}

        def run_one(r):
            res = recover(r, host_path_counts)
            with lock:
                out.write(json.dumps(res, ensure_ascii=False) + '\n')
                counted['n'] += 1
                if counted['n'] % 25 == 0:
                    print(f'progress {counted["n"]}/{len(pending)}', flush=True)

        list(pool.map(run_one, pending))

    with open(results_path) as f:
        tiers = {}
        for line in f:
            try:
                tiers[json.loads(line)['tier']] = tiers.get(json.loads(line)['tier'], 0) + 1
            except Exception:
                pass
    print('TIER_SUMMARY', json.dumps(tiers), flush=True)
    return 0


if __name__ == '__main__':
    sys.exit(main())
