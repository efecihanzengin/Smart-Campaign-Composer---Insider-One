---
name: export-session
description: Export a Claude Code session transcript into ai-sessions/ as scrubbed Markdown. Use when the user says "export session", "export this chat", "script it", "save the session", "session'ı export et", "session'ı kaydet", or asks to add a session to ai-sessions/.
---

# Export session

Converts a session's `.jsonl` transcript into `ai-sessions/NN-<slug>.md` with `scripts/transcript_to_md.py`, which removes secrets and personal data.

## 1. Find the transcript

Transcripts live in `~/.claude/projects/<project-dir>/<session-id>.jsonl`. `<project-dir>` is the repo's absolute path with every `/` replaced by `-`.

- **This session** (the default): its ID is the UUID in your scratchpad path. If you don't have one, use the most recently modified `.jsonl` in that folder, since the current session is still writing to it.
- **Another session**: use the ID the user gives. If they describe it instead ("the ticket 5 session"), list the `.jsonl` files with their modified times and grep them for the topic. Confirm with the user if more than one could match.

## 2. Pick the output file

- If a file in `ai-sessions/` already contains `<!-- session: <id> -->` for this session, overwrite that file. Re-exporting updates it rather than duplicating it.
- Otherwise use the next number after the highest `NN-` prefix in `ai-sessions/`. Take the slug from the name the user gave, or else from the session's main topic, in kebab case (e.g. `03-ticket-4-segment-rule`).
- Title: `AI session NN — <Topic>`.

## 3. Convert

Run from the repo root, so the script picks up `.scrub-terms`:

```bash
python3 -I scripts/transcript_to_md.py <transcript.jsonl> ai-sessions/NN-slug.md "AI session NN — Topic"
```

If `.scrub-terms` is missing, tell the user that fragments of email addresses other than the git email won't be redacted. Ask them for the terms to add (one per line) before publishing anything. Never write those terms anywhere that is committed.

## 4. Leak-check before reporting

Grep the output (case-insensitive) for:
- each part of `git config user.name`;
- each line of `.scrub-terms`;
- `sk-ant-`, `gh[pousr]_` followed by unredacted characters;
- the real home path.

Read the context of every hit:
- **Harmless:** the user's GitHub handle in repo URLs, and placeholder patterns inside grep commands (e.g. `/Users/` alone).
- **A leak:** anything else. Add the missing term to `.scrub-terms` (or fix the script) and re-run, until only harmless hits remain.

## 5. Report

Tell the user the source transcript path, the output file, its user and Claude turn counts, and any hits you judged harmless. Commit and push only if the user asked for it in this request. Otherwise ask.
