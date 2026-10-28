#!/usr/bin/env python3
"""Create a report-only workbook from job-digest-state using Python's stdlib."""
import csv
import hashlib
import json
import os
import re
import sys
import unicodedata
import zipfile
from collections import Counter, defaultdict
from datetime import datetime
from urllib.parse import urlsplit, urlunsplit
from xml.sax.saxutils import escape

ROOT = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), 'job-digest-state')
OUTPUT = os.path.join(os.path.dirname(ROOT), 'artifacts', 'job-digest-mining-report.xlsx')

# Job observations are keyed by normalized job URL to avoid counting one posting
# repeatedly across pipeline stages and daily debug exports.
jobs = {}
hn_sites = defaultdict(set)

# Company grouping is normalized beyond case: Unicode NFC, whitespace collapse,
# and edge-junk trimming, so 'NTT DATA Americas, Inc' and '…Inc.' share one row.
# Internal punctuation and brand marks (®, ™, °, brackets) are preserved.
EDGE_JUNK = re.compile(r"^[\s.,;:!?*|\u00b7~@&'\"\u2018\u2019\u201c\u201d\-\u2013\u2014]+|[\s.,;:!?*|\u00b7~@&'\"\u2018\u2019\u201c\u201d\-\u2013\u2014]+$")
PLACEHOLDER_NAMES = {'unknown', 'confidential', 'n/a', 'na', 'none', 'null', 'tbd',
                     'various', 'multiple', 'other', 'private', 'company', 'employer'}


def normalize_company_key(name):
    s = unicodedata.normalize('NFC', (name or '')).casefold()
    s = re.sub(r'[\s\u00a0\u200b\u200c\u200d\u2060\ufeff]+', ' ', s).strip()
    return EDGE_JUNK.sub('', s)


def deaccent_key(key):
    return re.sub(r'[^a-z0-9]', '', ''.join(
        c for c in unicodedata.normalize('NFD', key) if unicodedata.category(c) != 'Mn'))


company_buckets = {}


def company_bucket(name):
    key = normalize_company_key(name)
    if not key:
        return None, key
    bucket = company_buckets.get(key)
    if bucket is None:
        bucket = company_buckets[key] = {'variants': Counter(), 'locations': Counter(), 'urls': set(),
                                         'titles': set(), 'sources': set(), 'dates': set(),
                                         'provenance': set(), 'sample_url': '', 'hn_urls': set(),
                                         'hn_records': 0, 'raw_urls': set()}
    return bucket, key


def pick_display_name(variants):
    # Most-observed variant wins; prefer mixed-case spellings over ALL CAPS on
    # ties, then the shortest string, so 'ServiceNow' beats 'SERVICENOW'.
    return sorted(variants, key=lambda v: (-variants[v], 1 if (v[:1].isupper() and not v.isupper()) else 0, len(v)))[0]


def normalize_url(url):
    try:
        p = urlsplit((url or '').strip())
        if p.scheme not in ('http', 'https') or not p.hostname:
            return ''
        host = p.hostname.lower().removeprefix('www.')
        path = re.sub(r'/+', '/', p.path).rstrip('/')
        return urlunsplit(('https', host, path, '', ''))
    except Exception:
        return ''


def add_job(company, title, url, source, seen, provenance, location=''):
    company = (company or '').strip()
    title = (title or '').strip()
    location = (location or '').strip()
    normalized = normalize_url(url)
    bucket, _ = company_bucket(company)
    if bucket is not None:
        bucket['variants'][company] += 1
        if location:
            bucket['locations'][location] += 1
        if source:
            bucket['sources'].add(source.strip())
        if seen:
            bucket['dates'].add(seen[:10])
        bucket['provenance'].add(provenance)
        if normalized and not bucket['sample_url']:
            bucket['sample_url'] = normalized
    if not normalized:
        return
    item = jobs.setdefault(normalized, {'company': company, 'title': title, 'url': normalized,
                                        'sources': set(), 'dates': set(), 'provenance': set()})
    if company and not item['company']:
        item['company'] = company
    if title and not item['title']:
        item['title'] = title
    if source:
        item['sources'].add(source.strip())
    if seen:
        item['dates'].add(seen[:10])
    item['provenance'].add(provenance)


# Prefer first-stage fetch data as primary records; later stages repeat those jobs.
for base, _, files in os.walk(os.path.join(ROOT, 'production-artifacts')):
    if '01-fetched.json' not in files:
        continue
    path = os.path.join(base, 'artifacts', '01-fetched.json')
    run = os.path.basename(base)[:8]
    try:
        with open(path, encoding='utf-8') as f:
            data = json.load(f)
        for r in data.get('records', []):
            add_job(r.get('company'), r.get('title'), r.get('url'), r.get('source'),
                    r.get('posted_at') or run, 'fetched JSON', r.get('location'))
    except (OSError, ValueError):
        pass

# Diagnostics provide rows from runs whose artifact snapshots are absent, plus
# keep/reject reason context. Same normalized URL is merged with fetched records.
for filename in sorted(os.listdir(ROOT)):
    if not re.fullmatch(r'debug_\d{4}-\d{2}-\d{2}\.csv', filename):
        continue
    day = filename[6:16]
    try:
        with open(os.path.join(ROOT, filename), newline='', encoding='utf-8-sig') as f:
            for r in csv.DictReader(f):
                add_job(r.get('company'), r.get('title'), r.get('url'), r.get('source'),
                        r.get('posted_at') or day, 'debug CSV', r.get('location'))
    except (OSError, csv.Error):
        pass

# HN cache links may be a company career homepage rather than a job posting.
hn_entries = {}
hn_path = os.path.join(ROOT, 'hn-cache.json')
try:
    with open(hn_path, encoding='utf-8') as f:
        cache = json.load(f)
    for post_id, post in cache.items():
        for comment_id, comment in post.get('comments', {}).items():
            info = comment.get('info') or {}
            if comment.get('status') != 'positive' or not info.get('is_hiring'):
                continue
            company = (info.get('company') or '').strip()
            url = (info.get('url') or '').strip()
            norm = normalize_url(url)
            if not company:
                continue
            bucket, key = company_bucket(company)
            if bucket is not None:
                bucket['variants'][company] += 1
                location = (info.get('location') or '').strip()
                if location:
                    bucket['locations'][location] += 1
                bucket['sources'].add('HN')
                bucket['provenance'].add('hn-cache.json')
                title = (info.get('title') or '').strip()
                if title:
                    bucket['titles'].add(title)
                if norm:
                    bucket['hn_urls'].add(norm)
                    if not bucket['sample_url']:
                        bucket['sample_url'] = norm
                bucket['hn_records'] += 1
                bucket['raw_urls'].add(url)
            hn_sites[key].add(url)
            hn_entries[(key, url)] = {'company': company, 'url': url, 'normalized': norm,
                                      'title': (info.get('title') or '').strip(),
                                      'post_id': post_id, 'comment_id': comment_id}
except (OSError, ValueError):
    pass

# Aggregate deduplicated jobs by company/title/site.
company_jobs = defaultdict(dict)
title_jobs = defaultdict(dict)
site_jobs = defaultdict(dict)
for key, job in jobs.items():
    company, title, url = job['company'], job['title'], job['url']
    company_key = normalize_company_key(company)
    if company_key:
        company_jobs[company_key][key] = job
        bucket = company_buckets[company_key]
        bucket['urls'].add(url)
        if title:
            bucket['titles'].add(title)
    if title:
        title_jobs[title.casefold()][key] = job
    try:
        p = urlsplit(url)
        host = p.hostname.lower().removeprefix('www.')
        segs = [s for s in p.path.split('/') if s]
    except Exception:
        continue
    source_names = {s.lower() for s in job['sources']}
    ats_hosts = {
        'job-boards.greenhouse.io': ('Greenhouse', 1), 'boards.greenhouse.io': ('Greenhouse', 1),
        'jobs.lever.co': ('Lever', 1), 'jobs.ashbyhq.com': ('Ashby', 1),
        'jobs.jobvite.com': ('Jobvite', 1), 'jobs.smartrecruiters.com': ('SmartRecruiters', 1),
        'apply.workable.com': ('Workable', 1), 'jobs.workable.com': ('Workable', 1),
        'jobs.gem.com': ('Gem', 1), 'app.dover.io': ('Dover', 1),
    }
    ats = next((name for domain, (name, _) in ats_hosts.items() if host == domain or host.endswith('.' + domain)), '')
    workday = bool(re.search(r'\\.wd[0-9]+\\.myworkdayjobs\\.com$', host))
    icims = host.endswith('.icims.com')
    company_tokens = re.findall(r'[a-z0-9]+', company.casefold())
    company_tokens = [t for t in company_tokens if t not in {'inc', 'llc', 'ltd', 'co', 'company', 'corporation', 'corp', 'the', 'group'} and len(t) > 2]
    labels = host.split('.')
    domain_label = labels[-2] if len(labels) >= 2 else host
    company_domain = bool(company_tokens and any(t in domain_label for t in company_tokens))
    if ats:
        token = segs[0] if segs else ''
        root = f'https://{host}/{token}' if token else f'https://{host}'
        kind = ats
        confidence = 'High — recognized ATS host'
        flag = 'Confirm this account slug belongs to the named company.'
    elif workday:
        if not company_domain:
            continue
        root = f'https://{host}'
        kind = 'Workday tenant'
        confidence = 'Medium — company-matching tenant'
        flag = 'Verify tenant-to-company mapping; Workday host may be shared.'
    elif icims:
        if not company_domain:
            continue
        root = f'https://{host}'
        kind = 'iCIMS tenant'
        confidence = 'Medium — company-matching tenant'
        flag = 'Verify tenant-to-company mapping.'
    elif company_domain:
        root = f'https://{host}'
        kind = 'Company domain'
        confidence = 'Medium — domain resembles company name'
        flag = 'Confirm this is the company careers/jobs page, not a generic company page.'
    else:
        continue  # Skeptical default: exclude aggregators, search results, and unrelated hosts.
    skey = (normalize_company_key(company), root, kind)
    site_jobs[skey][key] = {'job': job, 'root': root, 'company': company, 'kind': kind,
                             'confidence': confidence, 'flag': flag, 'ckey': normalize_company_key(company)}

# Add HN candidate URLs as separate site suggestions.
for entry in hn_entries.values():
    url, norm = entry['url'], entry['normalized']
    if not norm:
        continue
    p = urlsplit(norm)
    host = p.hostname.lower().removeprefix('www.')
    company_tokens = [t for t in re.findall(r'[a-z0-9]+', entry['company'].casefold()) if t not in {'inc', 'llc', 'ltd', 'co', 'company', 'corporation', 'corp', 'the', 'group'} and len(t) > 2]
    domain_label = host.split('.')[-2] if len(host.split('.')) >= 2 else host
    ats_map = {'jobs.ashbyhq.com':'Ashby', 'jobs.lever.co':'Lever', 'job-boards.greenhouse.io':'Greenhouse', 'boards.greenhouse.io':'Greenhouse', 'apply.workable.com':'Workable', 'jobs.smartrecruiters.com':'SmartRecruiters', 'jobs.jobvite.com':'Jobvite'}
    ats = ats_map.get(host, '')
    if ats:
        segs = [s for s in p.path.split('/') if s]
        root = f'https://{host}/{segs[0]}' if segs else f'https://{host}'
        kind, confidence, flag = ats, 'Medium — HN evidence', 'Verify slug and board ownership.'
    elif company_tokens and any(t in domain_label for t in company_tokens):
        root = norm if p.path and p.path != '/' else f'https://{host}'
        kind, confidence, flag = 'Company domain', 'Medium — domain resembles company name', 'Confirm this is a careers/jobs page; URL may be a homepage.'
    else:
        continue  # HN URLs must look like an ATS or match the employer domain.
    skey = (normalize_company_key(entry['company']), root, kind)
    site_jobs[skey].setdefault('hn:' + entry['comment_id'], {'job': {'title': entry['title'], 'sources': {'HN'}, 'dates': set()},
      'root': root, 'company': entry['company'], 'kind': kind, 'confidence': confidence, 'flag': flag,
      'ckey': normalize_company_key(entry['company'])})

# Auto-merge lookalike groups (operator-approved): buckets whose deaccented
# alphanumeric keys collide become one bucket under the most-observed spelling.
# Folded-in names stay visible as 'Merged names' evidence on the surviving row.
merged_names = defaultdict(list)
canonical_of = {}
merge_targets = defaultdict(list)
for key in company_buckets:
    merge_targets[deaccent_key(key)].append(key)
for members in merge_targets.values():
    if len(members) < 2:
        continue
    canonical = min(members, key=lambda k: (-sum(company_buckets[k]['variants'].values()), k))
    for old in members:
        if old == canonical:
            continue
        src, dst = company_buckets[old], company_buckets[canonical]
        merged_names[canonical].append(pick_display_name(src['variants']))
        dst['variants'] += src['variants']
        dst['locations'] += src['locations']
        for field in ('urls', 'titles', 'sources', 'dates', 'provenance', 'hn_urls', 'raw_urls'):
            dst[field] |= src[field]
        dst['hn_records'] += src['hn_records']
        if not dst['sample_url']:
            dst['sample_url'] = src['sample_url']
        canonical_of[old] = canonical
        del company_buckets[old]
for old, canonical in canonical_of.items():
    if old in company_jobs:
        company_jobs[canonical].update(company_jobs.pop(old))
site_jobs = defaultdict(dict, (
    ((canonical_of.get(ckey, ckey), root, kind), obs)
    for (ckey, root, kind), obs in site_jobs.items()))

# Spreadsheet generation: minimal valid OOXML workbook with inline strings.
def col_name(n):
    out = ''
    while n:
        n, rem = divmod(n - 1, 26)
        out = chr(65 + rem) + out
    return out


def cell_xml(row, col, value, header=False):
    ref = f'{col_name(col)}{row}'
    style = ' s="1"' if header else ''
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return f'<c r="{ref}"{style}><v>{value}</v></c>'
    text = '' if value is None else str(value)
    preserve = ' xml:space="preserve"' if text.startswith(' ') or text.endswith(' ') or '\n' in text else ''
    return f'<c r="{ref}" t="inlineStr"{style}><is><t{preserve}>{escape(text)}</t></is></c>'


def sheet_xml(rows, widths=None):
    maxcols = max((len(r) for r in rows), default=1)
    cols = ''
    if widths:
        cols = '<cols>' + ''.join(f'<col min="{i}" max="{i}" width="{w}" customWidth="1"/>' for i, w in enumerate(widths, 1)) + '</cols>'
    body = []
    for ri, row in enumerate(rows, 1):
        cells = ''.join(cell_xml(ri, ci, v, ri == 1) for ci, v in enumerate(row, 1))
        body.append(f'<row r="{ri}">{cells}</row>')
    last = f'{col_name(maxcols)}{len(rows)}'
    return ('<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
            'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
            '<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>'
            f'{cols}<sheetData>{"".join(body)}</sheetData><autoFilter ref="A1:{last}"/>'
            '<sheetFormatPr defaultRowHeight="15"/></worksheet>')

# Display names: most-observed variant per (post-merge) company key.
display_names = {key: pick_display_name(bucket['variants']) for key, bucket in company_buckets.items()}
merged_display = {key: sorted(names, key=str.casefold) for key, names in merged_names.items()}

# Company rows
companies = []
for key, observations in company_jobs.items():
    bucket = company_buckets[key]
    display = display_names[key]
    srcs = set(bucket['sources'])
    dates = set(bucket['dates'])
    for job in observations.values():
        srcs.update(job['sources'])
        dates.update(job['dates'])
    companies.append([display, len(bucket['urls']), len(bucket['titles']), ', '.join(sorted(srcs)),
                      max(dates) if dates else '',
                      ' | '.join(sorted(bucket['variants'], key=str.casefold)),
                      ', '.join(sorted(bucket['provenance'])), '; '.join(merged_display.get(key, []))])
companies.sort(key=lambda r: (-r[1], r[0].casefold()))
company_rows = [['Company name (candidate)', 'Unique job URLs', 'Distinct titles', 'Observed sources', 'Latest posted date', 'Observed name variants', 'Evidence sources', 'Merged names']] + companies

# Step 1 identity review sheet: every observed name group, including HN-only names.
identity_records = sorted(company_buckets.items(), key=lambda item: display_names[item[0]].casefold())
identity_rows = [['Report company ID', 'Observed company name', 'Observed name variants', 'Unique job URLs', 'HN URLs', 'Distinct raw titles', 'Top locations', 'Sample job URL', 'First seen', 'Last seen', 'Sources', 'Evidence files', 'Merged names', 'Triage flags', 'Canonical company name (review)', 'Identity classification (review)', 'Official website (review)', 'Identity evidence / notes', 'Review status']]
for key, bucket in identity_records:
    display = display_names[key]
    junk_name = key in PLACEHOLDER_NAMES or len(display) > 80
    flags = []
    if any(v != v.strip() or EDGE_JUNK.search(v) for v in bucket['variants']):
        flags.append('Edge characters/spacing normalized')
    if key in PLACEHOLDER_NAMES:
        flags.append('Placeholder name?')
    if key.isdigit():
        flags.append('Numeric-only name?')
    if len(display) > 80:
        flags.append('Overlong name (>80 chars)')
    if not bucket['urls'] and not bucket['hn_urls']:
        flags.append('No job or HN URL observed')
    company_id = 'C' + hashlib.sha256(key.encode('utf-8')).hexdigest()[:10].upper()
    identity_rows.append([company_id, display,
                          ' | '.join(sorted(bucket['variants'], key=str.casefold)),
                          len(bucket['urls']), len(bucket['hn_urls']), len(bucket['titles']),
                          '; '.join(name for name, _ in bucket['locations'].most_common(3)),
                          bucket['sample_url'],
                          min(bucket['dates']) if bucket['dates'] else '',
                          max(bucket['dates']) if bucket['dates'] else '',
                          ', '.join(sorted(bucket['sources'])),
                          ', '.join(sorted(bucket['provenance'])),
                          '; '.join(merged_display.get(key, [])), '; '.join(flags),
                          '',
                          'Not a company name' if junk_name else '',
                          '', '',
                          'Excluded (junk name)' if junk_name else 'Not reviewed'])

# Step 2 site-evidence sheet: every observed URL grouped per company by class.
# ATS roots are the scrape-ready candidates; aggregators are discovery evidence;
# company-domain links are careers-page candidates pending verification.
ATS_PROVIDERS = {
    'job-boards.greenhouse.io': 'Greenhouse', 'boards.greenhouse.io': 'Greenhouse',
    'jobs.lever.co': 'Lever', 'jobs.ashbyhq.com': 'Ashby',
    'jobs.jobvite.com': 'Jobvite', 'jobs.smartrecruiters.com': 'SmartRecruiters',
    'apply.workable.com': 'Workable', 'jobs.workable.com': 'Workable',
    'jobs.gem.com': 'Gem', 'app.dover.io': 'Dover',
}
AGGREGATOR_HOSTS = {
    'adzuna.com', 'climatebase.org', 'monster.com', 'linkedin.com', 'indeed.com',
    'glassdoor.com', 'ziprecruiter.com', 'dice.com', 'jobgether.com', 'jobleads.com',
    'jobright.ai', 'bebee.com', 'learn4good.com', 'tealhq.com', 'clearancejobs.com',
    'flexjobs.com', 'jooble.org', 'lensa.com', 'theladders.com', 'himalayas.app',
    'remoterocketship.com', 'wellfound.com', 'remotive.com', 'virtualvocations.com',
    'careerbuilder.com', 'welcometothejungle.com', 'startup.jobs', 'remotejobs.org',
    'career.io', 'bandana.com', 'builtin.com', 'remote.co', 'dailyremote.com',
    'nordictechjobs.com', 'haystackapp.io', 'trabajo.org', 'jobs.polymer.co', 'join.com',
}
EVIDENCE_CLASS_ORDER = {'ATS board': 0, 'Company domain': 1, 'Aggregator': 2, 'Unclassified': 3, 'Malformed': 4}


def company_name_tokens(name):
    return [t for t in re.findall(r'[a-z0-9]+', normalize_company_key(name))
            if t not in {'inc', 'llc', 'ltd', 'co', 'company', 'corporation', 'corp', 'the', 'group'} and len(t) > 2]


def classify_evidence_url(raw, tokens):
    raw = (raw or '').strip()
    try:
        p = urlsplit(raw)
    except ValueError:
        p = None
    if not p or p.scheme not in ('http', 'https') or not p.hostname:
        return 'Malformed', raw, 'Discard — not a usable URL', ''
    host = p.hostname.lower().removeprefix('www.')
    segs = [s for s in p.path.split('/') if s]
    provider = next((n for d, n in ATS_PROVIDERS.items() if host == d or host.endswith('.' + d)), '')
    if not provider and re.search(r'\.wd\d+\.myworkdayjobs\.com$', host):
        provider = 'Workday'
    elif not provider and host.endswith('.icims.com'):
        provider = 'iCIMS'
    if provider in ('Workday', 'iCIMS'):
        return 'ATS board', f'https://{host}', 'Job-listing source candidate', 'Tenant may host multiple brands; confirm the mapping.'
    if host == 'grnh.se':
        return 'ATS board', f'https://{host}', 'Job-listing source candidate', 'Greenhouse short link — resolve the redirect to find the board root.'
    if provider:
        root = f'https://{host}/{segs[0]}' if segs else f'https://{host}'
        return 'ATS board', root, 'Job-listing source candidate', 'Confirm this slug belongs to the named company.'
    if any(host == d or host.endswith('.' + d) for d in AGGREGATOR_HOSTS):
        return 'Aggregator', f'https://{host}', 'Discovery evidence only — never a job-site candidate', ''
    labels = host.split('.')
    domain_label = labels[-2] if len(labels) >= 2 else host
    if tokens and any(t in domain_label for t in tokens):
        return 'Company domain', normalize_url(raw) or f'https://{host}', 'Careers-page candidate (verify)', 'Confirm this is a careers/jobs page, not a generic company page.'
    return 'Unclassified', f'https://{host}', 'Needs review — unknown third-party host', ''


evidence_groups = defaultdict(lambda: {'urls': set(), 'sources': set(), 'role': '', 'note': ''})
for key, observations in company_jobs.items():
    tokens = company_name_tokens(display_names[key])
    for job in observations.values():
        cls, root, role, note = classify_evidence_url(job['url'], tokens)
        group = evidence_groups[(key, cls, root)]
        group['urls'].add(job['url'])
        group['sources'] |= job['sources']
        group['role'], group['note'] = role, note
for key, bucket in company_buckets.items():
    tokens = company_name_tokens(display_names[key])
    for raw in bucket['raw_urls']:
        cls, root, role, note = classify_evidence_url(raw, tokens)
        group = evidence_groups[(key, cls, root)]
        group['urls'].add(normalize_url(raw) or raw)
        group['sources'].add('HN')
        group['role'], group['note'] = role, note
evidence_rows = [['Report company ID', 'Company', 'URL class', 'Role', 'URL / board root', 'Distinct URLs', 'Sample full URLs (up to 3)', 'Sources', 'Notes']]
for (key, cls, root), group in sorted(
        evidence_groups.items(),
        key=lambda item: (display_names[item[0][0]].casefold(), EVIDENCE_CLASS_ORDER.get(item[0][1], 9), item[0][2])):
    evidence_rows.append(['C' + hashlib.sha256(key.encode('utf-8')).hexdigest()[:10].upper(),
                          display_names[key], cls, group['role'], root, len(group['urls']),
                          '; '.join(sorted(group['urls'])[:3]),
                          ', '.join(sorted(group['sources'])), group['note']])

# Step 3 pilot verification sheet: results from artifacts/pilot-results/*.json.
PILOT_RESULTS_DIR = os.path.join(os.path.dirname(ROOT), 'artifacts', 'pilot-results')
PILOT_STATUSES = {'Verified — board confirms company', 'Verified — official-site link',
                  'Candidate — needs review', 'Unresolved — insufficient evidence',
                  'Unavailable — blocked or inaccessible'}
pilot_meta = {}
pilot_plan_path = os.path.join(os.path.dirname(ROOT), 'artifacts', 'pilot-companies.json')
try:
    with open(pilot_plan_path, encoding='utf-8') as f:
        for group in json.load(f).values():
            for entry in group:
                pilot_meta[entry['id']] = entry
except (OSError, ValueError):
    pass
pilot_records = []
for results_dir, pattern in ((PILOT_RESULTS_DIR, r'slice-\d+\.json'),
                             (os.path.join(os.path.dirname(ROOT), 'artifacts', 'rollout-results'), r'batch-\d+\.json')):
    if not os.path.isdir(results_dir):
        continue
    for name in sorted(os.listdir(results_dir)):
        if not re.fullmatch(pattern, name):
            continue
        try:
            with open(os.path.join(results_dir, name), encoding='utf-8') as f:
                pilot_records.extend(json.load(f))
        except (OSError, ValueError):
            pass
pilot_rows_data = []
seen_pilot_ids = set()
for rec in pilot_records:
    rid = rec.get('id', '')
    meta = pilot_meta.get(rid, {})
    status = rec.get('status', '') if rec.get('status', '') in PILOT_STATUSES else 'Unresolved — insufficient evidence'
    seen_pilot_ids.add(rid)
    pilot_rows_data.append([rid, rec.get('company', '') or meta.get('company', ''), meta.get('class', ''),
                            rec.get('official_site', ''), rec.get('careers_page', ''), rec.get('listing_source', ''),
                            rec.get('ats_provider', ''), status, rec.get('evidence', ''), rec.get('notes', ''),
                            datetime.utcnow().strftime('%Y-%m-%d')])
for rid, entry in sorted(pilot_meta.items()):
    if rid not in seen_pilot_ids:
        pilot_rows_data.append([rid, entry.get('company', ''), entry.get('class', ''), '', '', '', '',
                                'Unresolved — insufficient evidence', '', 'Missing from investigator output',
                                datetime.utcnow().strftime('%Y-%m-%d')])
pilot_rows_data.sort(key=lambda r: (r[2], r[1].casefold()))
verification_rows = [['Report company ID', 'Company', 'Pilot class', 'Official website', 'Careers page', 'Job-listing source', 'ATS/provider', 'Verification status', 'Evidence', 'Notes', 'Checked (UTC)']] + pilot_rows_data

# Title rows
titles = []
for key, observations in title_jobs.items():
    values = [j['title'] for j in observations.values() if j['title']]
    display = max(set(values), key=lambda x: (len(x), x.casefold())) if values else key
    co = sorted({j['company'] for j in observations.values() if j['company']}, key=str.casefold)
    src = sorted({s for j in observations.values() for s in j['sources']})
    dates = [d for j in observations.values() for d in j['dates']]
    titles.append([display, len(observations), len(co), ', '.join(src), max(dates) if dates else '', '; '.join(co[:20]),
                   'Raw posting title; contains rejected jobs too. Use filter/reason context before adopting as target title.'])
titles.sort(key=lambda r: (-r[1], r[0].casefold()))
title_rows = [['Job title (raw)', 'Unique job URLs', 'Companies', 'Observed sources', 'Latest posted date', 'Observed companies (first 20)', 'Review note']] + titles

# Career/job site rows
sites = []
for skey, observations in site_jobs.items():
    sample = next(iter(observations.values()))
    jobobs = [o['job'] for o in observations.values()]
    titles_here = sorted({o.get('title', '') for o in jobobs if o.get('title')}, key=str.casefold)
    sources_here = sorted({s for o in jobobs for s in o.get('sources', set())})
    dates_here = [d for o in jobobs for d in o.get('dates', set())]
    company_display = display_names.get(sample.get('ckey', ''), sample['company'])
    sites.append([company_display, sample['root'], sample['kind'], len(observations), ', '.join(sources_here),
                  '; '.join(titles_here[:10]), sample['confidence'], sample['flag'], max(dates_here) if dates_here else ''])
sites.sort(key=lambda r: (-r[3], r[0].casefold(), r[1]))
site_rows = [['Company', 'Candidate board/site root', 'Site type', 'Unique observed job URLs', 'Sources', 'Sample titles (up to 10)', 'Confidence', 'Review flag', 'Latest posted date']] + sites

# Combined import-review view aligned with internal/db job_boards columns.
# Company names remain candidate values; accepted companies can later be linked
# to one or more discovered board rows.
company_sites = [['Company name (candidate)', 'board_url', 'ats_type', 'suggested_status', 'unique_job_urls', 'sources', 'latest_posted_date', 'confidence', 'metadata/review_note']]
for row in sites:
    company, board_url, kind, count, sources_seen, sample_titles, confidence, flag, latest = row
    ats = kind.lower() if kind.lower() in ('greenhouse', 'lever', 'ashby', 'jobvite', 'smartrecruiters', 'workable', 'gem', 'dover') else ('workday' if kind == 'Workday tenant' else ('icims' if kind == 'iCIMS tenant' else ''))
    company_sites.append([company, board_url, ats, 'discovered', count, sources_seen, latest, confidence,
                         f'site_type={kind}; sample_titles={sample_titles}; review={flag}'])
company_sites[1:] = sorted(company_sites[1:], key=lambda r: (r[0].casefold(), r[1]))

# Count unique values, caveated as extracted candidates rather than verified entities.
raw_name_count = sum(len(b['variants']) for b in company_buckets.values())
summary_rows = [
 ['Job-digest-state mining report', 'Generated (UTC)', datetime.utcnow().strftime('%Y-%m-%d %H:%M UTC')],
 ['Purpose', 'Report-only extraction; no source data or application database modified.'],
 ['Source coverage', 'Pipeline 01-fetched snapshots, daily debug CSVs, and positive hiring records from hn-cache.json.'],
 ['Source directory', ROOT],
 ['Unique job URLs', len(jobs)],
 ['Observed company name groups (post-merge)', len(company_buckets)],
 ['Distinct raw company strings', raw_name_count],
 ['Names auto-merged into other groups', sum(len(v) for v in merged_names.values())],
 ['Names excluded as junk (operator-approved)', sum(1 for k, b in company_buckets.items() if k in PLACEHOLDER_NAMES or len(display_names[k]) > 80)],
 ['Companies with observed job URLs', len(companies)],
 ['Distinct raw job-title strings', len(titles)],
 ['Company/site candidates', len(sites)],
 ['Step 2: classified URL evidence groups', len(evidence_rows) - 1],
 ['HN hiring records', len(hn_entries)],
 ['Pilot: companies verified — board confirms company', sum(1 for r in pilot_rows_data if r[7] == 'Verified — board confirms company')],
 ['Pilot: companies verified — official-site link', sum(1 for r in pilot_rows_data if r[7] == 'Verified — official-site link')],
 ['Pilot: companies candidate — needs review', sum(1 for r in pilot_rows_data if r[7] == 'Candidate — needs review')],
 ['Pilot: companies unresolved', sum(1 for r in pilot_rows_data if r[7] == 'Unresolved — insufficient evidence')],
 ['Pilot: companies unavailable (blocked)', sum(1 for r in pilot_rows_data if r[7] == 'Unavailable — blocked or inaccessible')],
 ['Pipeline snapshots', sum(1 for base, _, files in os.walk(os.path.join(ROOT, 'production-artifacts')) if '01-fetched.json' in files)],
 ['Debug CSV files', sum(1 for f in os.listdir(ROOT) if re.fullmatch(r'debug_\d{4}-\d{2}-\d{2}\.csv', f))],
 ['Method: de-duplication', 'Jobs deduplicated by HTTPS URL host/path (query and fragment removed); stage snapshots after 01-fetched excluded to avoid double-counting.'],
 ['Method: company names', 'Grouped by Unicode NFC + casefold + collapsed whitespace + edge-junk trim (leading/trailing . , * | @ etc.). Internal punctuation and brand marks (®, ™, °, brackets) preserved. Display name = most-observed variant.'],
 ['Method: suggested merges', 'Groups whose deaccented alphanumeric keys collide (e.g. U.S. Bank / US Bank, Ōura / Oura) are auto-merged under the most-observed spelling; folded-in names remain visible in the Merged names column.'],
 ['Method: sites', 'Allow recognized ATS hosts and domains whose second-level label resembles the company name; exclude unrelated hosts, aggregators, and search/referral domains. Workday/iCIMS tenants require a company-name match. HN URLs follow the same rule.'],
 ['Site review rule', 'A company-matching domain is only a candidate, not a verified careers page. Confirm that the URL is specifically a job board/careers page before import.'],
 ['Important caveat', 'Candidate names/titles/sites are observed data, not human-verified canonical entities. Counts do not imply endorsement or active availability.'],
 ['Titles caveat', 'Titles include all jobs found in fetch and diagnostic data, including rejected postings; filter semantically before seeding role suggestions.'],
 ['Site caveat', 'A posting URL reveals an ATS board, not necessarily the employer’s own career homepage. Generic ATS tenants and third-party aggregators require review.'],
 ['Recommended review', 'Start with the 1 - Company Identity tab: triage flags first, then suggested merges, then canonical name/classification/official-website review fields.'],
]

os.makedirs(os.path.dirname(OUTPUT), exist_ok=True)
workbook = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Summary" sheetId="1" r:id="rId1"/><sheet name="Companies" sheetId="2" r:id="rId2"/><sheet name="Job Titles" sheetId="3" r:id="rId3"/><sheet name="Job Sites" sheetId="4" r:id="rId4"/><sheet name="Company Sites" sheetId="5" r:id="rId5"/><sheet name="1 - Company Identity" sheetId="6" r:id="rId6"/><sheet name="2 - Site Evidence" sheetId="7" r:id="rId7"/><sheet name="3 - Site Verification" sheetId="8" r:id="rId8"/></sheets></workbook>'''
styles = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF1F4E78"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="2" borderId="0" xfId="0" applyFont="1" applyFill="1"/></cellXfs></styleSheet>'''
rels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet3.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet4.xml"/><Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet5.xml"/><Relationship Id="rId6" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet6.xml"/><Relationship Id="rId7" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet7.xml"/><Relationship Id="rId8" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet8.xml"/><Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>'''
rootrels = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>'''
ct = '''<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>''' + ''.join(f'<Override PartName="/xl/worksheets/sheet{i}.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>' for i in range(1,9)) + '</Types>'
with zipfile.ZipFile(OUTPUT, 'w', zipfile.ZIP_DEFLATED) as z:
    z.writestr('[Content_Types].xml', ct)
    z.writestr('_rels/.rels', rootrels)
    z.writestr('xl/workbook.xml', workbook)
    z.writestr('xl/_rels/workbook.xml.rels', rels)
    z.writestr('xl/styles.xml', styles)
    for i, rows in enumerate((summary_rows, company_rows, title_rows, site_rows, company_sites, identity_rows, evidence_rows, verification_rows), 1):
        widths = ([38, 120, 25] if i == 1 else [36, 20, 20, 34, 22, 65, 45, 40] if i == 2 else [55, 20, 15, 34, 22, 100, 90] if i == 3 else [36, 75, 25, 22, 34, 80, 40, 95, 22] if i == 4 else [36, 75, 18, 18, 16, 30, 20, 60, 40, 95, 22] if i == 5 else [13, 34, 45, 11, 9, 12, 32, 45, 12, 12, 20, 24, 40, 34, 30, 26, 34, 36, 14] if i == 6 else [13, 30, 16, 44, 60, 11, 80, 24, 46] if i == 7 else [13, 26, 14, 34, 34, 44, 15, 30, 60, 50, 12])
        z.writestr(f'xl/worksheets/sheet{i}.xml', sheet_xml(rows, widths))
print(f'Created {OUTPUT}')
print(f'Unique job URLs: {len(jobs):,}; name groups: {len(company_buckets):,} (from {raw_name_count:,} raw strings; '
      f'{sum(len(v) for v in merged_names.values()):,} auto-merged); title strings: {len(titles):,}; '
      f'site candidates: {len(sites):,}; HN records: {len(hn_entries):,}')
