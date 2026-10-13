# Mockups — UI iteration flow

Two kinds of files live here, with strict ownership rules:

## Operator-owned mockups (read-only for the agent)

Sketch/markup files you create (Krita/Xournal++ exports, PNG or flattened PSD/XCF
exports). The agent may **read** these but must **never modify, overwrite, or
delete** anything matching this set.

## Agent responses

When the agent implements a mockup, it copies the generated page capture here:

- First implementation: `<mockup-name>-response.png`
- Update after your next markup round: `<mockup-name>-response1.png`
- Further rounds: `<mockup-name>-response2.png`, `-response3.png`, …

The agent **never overwrites** a response file: every round gets a fresh number,
so your annotations always reference an immutable file. Source-of-truth captures
remain in `artifacts/ui-screenshots/` (regenerable via
`node scripts/ui-preview/generate-screenshots.mjs`); the files here are review
copies for the annotation loop.

## Loop

1. Operator drops/sketches `<mockup-name>.png` here and tells the agent.
2. Agent implements and writes `<mockup-name>-response.png`.
3. Operator annotates the response (on a new layer) and exports
   `<mockup-name>-edits.png` (any name works; edits files are operator-owned).
4. Agent reads the edits, updates code, writes `<mockup-name>-response1.png`.
5. Repeat from 3.
