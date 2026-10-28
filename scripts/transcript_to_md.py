"""Convert a Claude Code session transcript (.jsonl) into a readable, scrubbed Markdown file.

Usage: python3 -I scripts/transcript_to_md.py <transcript.jsonl> <out.md> "<title>"

Transcripts live in ~/.claude/projects/<project-dir>/<session-id>.jsonl. Personal data to redact
(name, email, username, home directory) is read from git config and the environment at run time,
so none of it is stored in this script. Extra terms go in SCRUB_TERMS or the gitignored .scrub-terms file.
"""
import getpass
import json
import os
import re
import subprocess
import sys


def git_config(key):
    try:
        return subprocess.run(["git", "config", key], capture_output=True, text=True).stdout.strip()
    except OSError:
        return ""


def build_redactions():
    rules = [
        (re.compile(r"sk-ant-[A-Za-z0-9_\-]+"), "[REDACTED_API_KEY]"),
        (re.compile(r"\b(gh[pousr]_)[A-Za-z0-9*]{10,}"), r"\1[REDACTED_TOKEN]"),
        (re.compile(r"(ANTHROPIC_API_KEY\s*=\s*)(?!\s|$|`)\S+"), r"\1[REDACTED]"),
        (re.compile(r"[A-Za-z0-9._%+\-]+@(?!anthropic\.com)[A-Za-z0-9.\-]+\.[A-Za-z]{2,}"), "[redacted-email]"),
    ]
    # Also redact parts of the name and email, which show up in commands like grep patterns.
    # Name parts match in any case but only where no letter touches them, so a GitHub handle that
    # runs them together stays intact while grep patterns like "[a-z]*part[a-z]*" or a regex
    # "\bPart\b" (whose "b" is a letter) are caught.
    name = git_config("user.name")
    if name:
        rules.append((re.compile(re.escape(name), re.I), "[redacted name]"))
        for part in name.split():
            if len(part) >= 3:
                bounded = r"(?:(?<=\\b)|(?<![^\W\d_]))" + re.escape(part) + r"(?=\\b|(?![^\W\d_]))"
                rules.append((re.compile(bounded, re.I), "[redacted name]"))
    # Extra terms (e.g. another email's local part) come from SCRUB_TERMS="a,b" and from the gitignored
    # .scrub-terms file (one per line, # for comments), so they never live in the repo.
    extra = [t.strip() for t in os.environ.get("SCRUB_TERMS", "").split(",") if t.strip()]
    if os.path.exists(".scrub-terms"):
        extra += [l.strip() for l in open(".scrub-terms") if l.strip() and not l.lstrip().startswith("#")]
    for email_local in [git_config("user.email").partition("@")[0]] + extra:
        if not email_local:
            continue
        prefix = re.match(r"[A-Za-z]{4,}", email_local)
        for token in sorted({email_local, prefix.group(0) if prefix else email_local}, key=len, reverse=True):
            rules.append((re.compile(re.escape(token), re.I), "[redacted]"))
    rules += [
        (re.compile(re.escape(os.path.expanduser("~"))), "~"),
        (re.compile(r"/private/tmp/claude-\d+/[^\s`'\")]+"), "<scratchpad>"),
        (re.compile(r"\b" + re.escape(getpass.getuser()) + r"\b"), "[user]"),
    ]
    return rules


REDACTIONS = build_redactions()
SYSTEM_REMINDER = re.compile(r"<system-reminder>.*?</system-reminder>", re.S)
PASTED = re.compile(r"</?pasted_content[^>]*>")
SKIP_TOOLS = {"mcp__ccd_turn__update_plan", "mcp__ccd_turn__wrap_up"}
MAX_RESULT_LINES = 40


def scrub(text):
    for pattern, repl in REDACTIONS:
        text = pattern.sub(repl, text)
    return text


def clean_user_text(text):
    text = SYSTEM_REMINDER.sub("", text)
    text = PASTED.sub("", text)
    if "<local-command-caveat>" in text:
        return None
    m = re.search(r"<command-name>(.*?)</command-name>", text, re.S)
    if m:
        args = re.search(r"<command-args>(.*?)</command-args>", text, re.S)
        return f"`{m.group(1).strip()}{' ' + args.group(1).strip() if args and args.group(1).strip() else ''}`"
    m = re.search(r"<local-command-stdout>(.*?)</local-command-stdout>", text, re.S)
    if m:
        return f"_(command output: {m.group(1).strip()})_"
    m = re.search(r"<bash-input>(.*?)</bash-input>", text, re.S)
    if m:
        out = re.search(r"<bash-stdout>(.*?)</bash-stdout>", text, re.S)
        tail = "\n".join(out.group(1).strip().splitlines()[-12:]) if out else ""
        return f"Ran in terminal:\n\n```\n{m.group(1).strip()}\n```\n\n<details><summary>Output (last lines)</summary>\n\n```\n{tail}\n```\n</details>"
    return text.strip() or None


def truncate(text):
    lines = text.splitlines()
    if len(lines) > MAX_RESULT_LINES:
        lines = lines[:MAX_RESULT_LINES] + [f"… ({len(lines) - MAX_RESULT_LINES} more lines)"]
    return "\n".join(lines)


def result_text(block):
    c = block.get("content")
    if isinstance(c, list):
        c = "\n".join(b.get("text", "") for b in c if b.get("type") == "text")
    return SYSTEM_REMINDER.sub("", c or "").strip()


def describe_tool(block):
    name, inp = block["name"], block.get("input", {})
    if name == "Bash":
        return f"**Bash** — {inp.get('description', '')}\n\n```bash\n{inp.get('command', '')}\n```"
    if name == "Write":
        n = inp.get("content", "").count("\n") + 1
        return f"**Write** `{inp.get('file_path')}` ({n} lines)"
    if name == "Edit":
        return f"**Edit** `{inp.get('file_path')}`"
    if name == "Read":
        return f"**Read** `{inp.get('file_path')}`"
    if name == "Skill":
        return f"**Skill** `{inp.get('skill')}`"
    if name == "AskUserQuestion":
        qs = "\n".join(
            f"- {q['question']} — options: " + ", ".join(o["label"] for o in q["options"])
            for q in inp.get("questions", [])
        )
        return f"**AskUserQuestion**\n\n{qs}"
    return f"**{name}**"


def main(src, dst, title):
    records = [json.loads(line) for line in open(src)]
    pending = {}  # tool_use_id -> tool name, for pairing results
    session_id = os.path.splitext(os.path.basename(src))[0]
    out = [f"# {title}", "", f"<!-- session: {session_id} -->", "",
           "Claude Code session export (desktop app). Thinking blocks and internal UI tool calls are omitted; "
           "tool outputs are truncated; secrets and personal data are redacted.", ""]
    last_role = None
    for o in records:
        if o.get("isSidechain") or o.get("type") not in ("user", "assistant"):
            continue
        msg = o.get("message") or {}
        content = msg.get("content")
        if o["type"] == "user":
            if o.get("isMeta"):
                m = re.search(r"skills/[^/]+/([^/\s]+)", content if isinstance(content, str) else
                              " ".join(b.get("text", "") for b in content if b.get("type") == "text"))
                out += [f"_(skill instructions loaded: {m.group(1) if m else 'skill'})_", ""]
                continue
            if isinstance(content, str):
                content = [{"type": "text", "text": content}]
            for b in content:
                if b.get("type") == "text":
                    text = clean_user_text(b["text"])
                    if text:
                        out += ["---", "", "## 🧑 User", "", text, ""]
                        last_role = "user"
                elif b.get("type") == "tool_result":
                    tool = pending.pop(b.get("tool_use_id"), None)
                    if tool in SKIP_TOOLS or tool is None:
                        continue
                    text = result_text(b)
                    if tool in ("Write", "Edit", "Skill") and not b.get("is_error"):
                        continue
                    if not text:
                        continue
                    label = "Error" if b.get("is_error") else "Result"
                    out += [f"<details><summary>{label}</summary>", "", "```", truncate(text), "```", "</details>", ""]
        else:
            for b in content or []:
                t = b.get("type")
                if t == "tool_use":
                    pending[b["id"]] = b["name"]
                    if b["name"] in SKIP_TOOLS:
                        continue
                if t == "text" and b["text"].strip() or t == "tool_use":
                    if last_role != "assistant":
                        out += ["---", "", "## 🤖 Claude", ""]
                        last_role = "assistant"
                    out += [b["text"].strip() if t == "text" else describe_tool(b), ""]
    open(dst, "w").write(scrub("\n".join(out)))


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    main(*sys.argv[1:4])
