#!/usr/bin/env python3
"""Create a Search B-only frozen careers worklist and fixed 40-row batch slices.

This reads the Search B workbook and the Search B batch-003 checkpoint only.
It intentionally does not read Search A files or the workbook's existing status fields.
"""
from pathlib import Path
import json
import re
import zipfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
BASE = ROOT / "artifacts/search B/company-list-sanity"
XLSX = BASE / "company-list-cross-reference.xlsx"
REPORT = BASE / "careers-batch-003.md"
OUT = BASE / "careers-wave-001"
NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}

with zipfile.ZipFile(XLSX) as z:
    workbook = ET.fromstring(z.read("xl/workbook.xml"))
    relroot = ET.fromstring(z.read("xl/_rels/workbook.xml.rels"))
    rels = {r.attrib["Id"]: r.attrib["Target"] for r in relroot}
    sheet_ref = workbook.find("m:sheets", NS)[0].attrib[
        "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"
    ]
    sheet_path = rels[sheet_ref].lstrip("/")
    if not sheet_path.startswith("xl/"):
        sheet_path = "xl/" + sheet_path
    sheet = ET.fromstring(z.read(sheet_path))
    shared = []
    if "xl/sharedStrings.xml" in z.namelist():
        ssroot = ET.fromstring(z.read("xl/sharedStrings.xml"))
        shared = ["".join(t.text or "" for t in si.findall(".//m:t", NS)) for si in ssroot.findall("m:si", NS)]

rows = []
for row in sheet.findall(".//m:sheetData/m:row", NS)[1:]:
    d = {}
    for cell in row.findall("m:c", NS):
        col = "".join(ch for ch in cell.attrib["r"] if ch.isalpha())
        v = cell.find("m:v", NS)
        text = v.text if v is not None else "".join(t.text or "" for t in cell.findall(".//m:t", NS))
        if cell.attrib.get("t") == "s" and text:
            text = shared[int(text)]
        d[col] = text or ""
    if d.get("A") and d.get("B"):
        rows.append({
            "source_id": d["A"],
            "observed_name": d["B"],
            "name_variants": d.get("C", ""),
            "location_context": d.get("F", ""),
            "sources": d.get("G", ""),
            "site_evidence_classes": d.get("H", ""),
            "site_evidence_roots": d.get("I", ""),
        })

report = REPORT.read_text()
completed = set(re.findall(r"^\|\s*\|?\s*(C[0-9A-F]{10,11})\s*\|", report, re.M))
unique = {}
for r in rows:
    unique.setdefault(r["source_id"], r)
worklist = [r for r in unique.values() if r["source_id"] not in completed]
OUT.mkdir(parents=True, exist_ok=True)
(OUT / "frozen-worklist.json").write_text(json.dumps(worklist, ensure_ascii=False, indent=2) + "\n")

batches = [worklist[i:i+40] for i in range(0, min(800, len(worklist)), 40)]
for i, batch in enumerate(batches, 1):
    (OUT / f"batch-{i:02d}.json").write_text(json.dumps(batch, ensure_ascii=False, indent=2) + "\n")
print(json.dumps({"workbook_unique_source_rows": len(unique), "batch003_completed_ids": len(completed), "remaining_rows": len(worklist), "wave_batches_created": len(batches), "rows_in_wave": sum(map(len, batches)), "last_batch_size": len(batches[-1]) if batches else 0, "output": str(OUT.relative_to(ROOT))}, indent=2))
