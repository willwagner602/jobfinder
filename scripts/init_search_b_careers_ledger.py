#!/usr/bin/env python3
"""Initialize Search B-only career-search accounting; never reads Search A."""
from pathlib import Path
import json
import sqlite3

ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT / "artifacts/search B/company-list-sanity"
WAVE = BASE / "careers-wave-001"
DB = BASE / "search-b-careers-ledger.sqlite3"
CAP = 150_000

with open(WAVE / "frozen-worklist.json", encoding="utf-8") as f:
    worklist = json.load(f)
with open(WAVE / "batch-01.json", encoding="utf-8") as f:
    wave = []
    for i in range(1, 21):
        wave.extend(json.load(open(WAVE / f"batch-{i:02d}.json", encoding="utf-8")))

con = sqlite3.connect(DB)
con.execute("PRAGMA foreign_keys=ON")
con.executescript("""
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS reservations (
  reservation_id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id TEXT NOT NULL,
  observed_name TEXT NOT NULL,
  batch_no INTEGER NOT NULL,
  purpose TEXT NOT NULL,
  query TEXT NOT NULL,
  reserved_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  status TEXT NOT NULL DEFAULT 'reserved',
  provider TEXT,
  outcome TEXT,
  result_count INTEGER,
  error TEXT
);
""")
meta = dict(con.execute("SELECT key,value FROM meta"))
if meta and meta.get("cap") != str(CAP):
    raise SystemExit(f"Existing Search B ledger cap mismatch: {meta.get('cap')}")
with con:
    con.execute("INSERT OR IGNORE INTO meta(key,value) VALUES('project','Search B company-careers research')")
    con.execute("INSERT OR IGNORE INTO meta(key,value) VALUES('cap',?)", (str(CAP),))
    con.execute("INSERT OR IGNORE INTO meta(key,value) VALUES('scope','Search B only; Search A excluded')")
    con.execute("INSERT OR IGNORE INTO meta(key,value) VALUES('wave_001_rows',?)", (str(len(wave)),))
    existing = {r[0] for r in con.execute("SELECT source_id FROM reservations WHERE purpose='wave001-initial-query'")}
    for idx, row in enumerate(wave):
        if row['source_id'] in existing:
            continue
        q = f"{row['observed_name']} careers"
        loc = row.get('location_context','').strip()
        if loc:
            q += f" {loc}"
        con.execute("INSERT INTO reservations(source_id,observed_name,batch_no,purpose,query) VALUES(?,?,?,?,?)",
                    (row['source_id'],row['observed_name'],idx//40+1,'wave001-initial-query',q))
reserved = con.execute("SELECT COUNT(*) FROM reservations").fetchone()[0]
print(json.dumps({"ledger":str(DB.relative_to(ROOT)),"search_b_cap":CAP,"wave001_rows":len(wave),"reserved_initial_queries":reserved,"available_after_reservations":CAP-reserved,"scope":"Search B only; does not read or write Search A"},indent=2))
