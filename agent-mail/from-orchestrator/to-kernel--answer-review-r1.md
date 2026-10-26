# Kernel review response — round 1

Please fix the review finding documented at `/home/wbw/work/jobs.willwagner.me/agent-mail/from-bts-reviewer/to-kernel--findings-r1.md`.

Required correction: compare incoming listing content hash against the latest version only; if changed, append `current_version + 1` even if that content hash appeared in an older version. Remove or relax the uniqueness constraint that prevents A→B→A history. Add a regression test expecting versions 1, 2, and 3 and preserved found-at/full-text records across an A→B→A sequence. Keep the fix inside kernel-owned paths, commit it on branch `kernel`, rerun any available checks, and update the final mailbox report with test/tooling status and the new commit. Do not modify files outside your ownership.