#!/usr/bin/env python3
"""Lane 1: confirm/enrich candidate rows via public ATS JSON APIs.
Supported: Greenhouse, Lever, Ashby, SmartRecruiters, Workday (CXS)."""
import json, os, re, sys, time
import urllib.request, urllib.error
from urllib.parse import urlparse

BASE = 'artifacts/search B/company-list-sanity'
QUEUES = {
    'fortune': json.load(open(BASE + '/fortune-run/remedy-queues.json')),
    'searchA': json.load(open('artifacts/search A/remedy-queues.json')),
}
OUT = BASE + '/remedy-ats-results.json'
UA = {'User-Agent': 'wb-jobs-research/1.0 (research bot)'}

def http_json(url, method='GET', body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method,
                                 headers={**UA, 'Content-Type': 'application/json',
                                          'Accept': 'application/json'})
    with urllib.request.urlopen(req, timeout=20) as resp:
        return resp.status, json.loads(resp.read().decode('utf-8', 'replace'))

def parse(url):
    h = urlparse(url).netloc.lower()
    path = [p for p in urlparse(url).path.split('/') if p]
    if 'greenhouse.io' in h:
        slug = path[1] if path and path[0] in ('job-boards', 'boards') else (path[0] if path else '')
        return ('greenhouse', slug) if slug else None
    if 'lever.co' in h:
        return ('lever', path[0]) if path else None
    if 'ashbyhq.com' in h:
        return ('ashby', path[-1] if path else None)
    if 'smartrecruiters.com' in h:
        return ('smartrecruiters', path[0]) if path else None
    if 'myworkdayjobs.com' in h or 'myworkdaysite.com' in h:
        # host IS the tenant host: {tenant}.wd{n}.myworkdayjobs.com; site from path,
        # skipping locale segments.
        tenant = h.split('.')[0]
        ps = [p for p in path if not re.fullmatch(r'en-[A-Z]{2}', p) and p != 'job']
        site = ps[0] if ps else None
        return ('workday', f'{h}/{site}') if site else None
    return None

def query(kind, slug):
    if kind == 'greenhouse':
        s, js = http_json(f'https://boards-api.greenhouse.io/v1/boards/{slug}/jobs')
        return {'openings': len(js.get('jobs', [])), 'total': js.get('meta', {}).get('total'),
                'sample': [j.get('title') for j in js.get('jobs', [])[:3]]}
    if kind == 'lever':
        s, js = http_json(f'https://api.lever.co/v0/postings/{slug}?mode=json')
        return {'openings': len(js), 'sample': [j.get('text') for j in js[:3]]}
    if kind == 'ashby':
        s, js = http_json(f'https://api.ashbyhq.com/posting-api/job-board/{slug}')
        return {'openings': len(js.get('jobs', [])), 'sample': [j.get('title') for j in js.get('jobs', [])[:3]]}
    if kind == 'smartrecruiters':
        s, js = http_json(f'https://api.smartrecruiters.com/v1/companies/{slug}/postings?limit=10')
        return {'openings': js.get('totalFound'), 'sample': [p.get('name') for p in js.get('content', [])[:3]]}
    if kind == 'workday':
        host, site = slug.split('/', 1)
        s, js = http_json(f'https://{host}/wday/cxs/{host.split(".")[0]}/{site}/jobs',
                          method='POST', body={'appliedFacets': {}, 'limit': 20, 'offset': 0, 'searchText': ''})
        return {'openings': js.get('total'), 'sample': [j.get('title') for j in js.get('jobPostings', [])[:3]]}
    raise ValueError(kind)

def main():
    results = []
    for lane, q in QUEUES.items():
        for rec in q['with_ats']:
            out = {**rec, 'lane': lane, 'ats_results': []}
            for u in rec['ats_urls']:
                parsed = parse(u)
                if not parsed:
                    continue
                kind, slug = parsed
                try:
                    res = query(kind, slug)
                    out['ats_results'].append({'url': u, 'kind': kind, 'slug': slug,
                                               'ok': True, **res})
                except Exception as e:  # noqa: BLE001
                    out['ats_results'].append({'url': u, 'kind': kind, 'slug': slug,
                                               'ok': False, 'error': repr(e)[:120]})
                time.sleep(0.3)
            confirmed = [a for a in out['ats_results'] if a.get('ok') and (a.get('openings') or 0) > 0]
            out['confirmed'] = bool(confirmed)
            out['openings'] = max((a.get('openings') or 0) for a in confirmed) if confirmed else 0
            results.append(out)
            print(f"{lane} {rec['source_id']} {rec['name'][:30]:30s} "
                  f"{'CONFIRMED openings=' + str(out['openings']) if confirmed else 'no-confirmation'}",
                  flush=True)
    json.dump(results, open(OUT, 'w'), indent=1)
    conf = sum(1 for r in results if r['confirmed'])
    print(json.dumps({'rows': len(results), 'confirmed': conf}))
    return 0

if __name__ == '__main__':
    sys.exit(main())
