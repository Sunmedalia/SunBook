"""Small, dependency-free Markdown utilities for SunBook's snapshot API."""

import html
import re
import unicodedata

from markdown_format import format_document
from sunbook_plugin import Plugin, PluginError, selected_text

BEGIN = "<!-- sunbook:toc -->"
END = "<!-- /sunbook:toc -->"
ATX = re.compile(r"^ {0,3}(#{1,6})(?:[ \t]+(.*)|$)")
SETEXT = re.compile(r"^ {0,3}(=+|-+)[ \t]*$")
FENCE = re.compile(r"^[ \t]*(`{3,}|~{3,})(.*)$")
LIST = re.compile(r"^([ \t]*)(?:[-+*]|[0-9]+[.)])[ \t]+")
TASK = re.compile(r"^[ \t]*(?:[-+*]|[0-9]+[.)])[ \t]+\[([ xX])\](?:[ \t]+(.*)|[ \t]*$)")


def _mask_comments(line, inside):
    """Keep character positions intact for checkbox byte offsets."""
    visible = list(line)
    cursor = 0
    while cursor < len(line):
        start = cursor if inside else line.find("<!--", cursor)
        if start < 0:
            break
        end = line.find("-->", start + (0 if inside else 4))
        stop = len(line) if end < 0 else end + 3
        visible[start:stop] = " " * (stop - start)
        inside = end < 0
        cursor = stop
    return "".join(visible), inside


class IncompleteMarkdown(PluginError):
    """Unsafe to format yet, but the source can still be previewed."""


def _scan(document, strict_toc=False, strict_format=False):
    if document is None:
        raise PluginError("Open a Markdown document first.")
    text = document["text"]
    if len(text.encode("utf-8")) > 1024 * 1024:
        raise PluginError("Markdown Tools supports documents up to 1 MiB.")
    lines = text.split("\n")
    rows, markers = [], []
    byte_offset = char_offset = 0
    frontmatter = bool(lines and lines[0].lstrip("\ufeff") == "---")
    fence = None
    comment = False
    list_stack = []
    html_end = None
    html_block = False
    for index, line in enumerate(lines):
        row = {
            "text": line,
            "visible": "",
            "byte": byte_offset,
            "start": char_offset,
            "end": char_offset + len(line),
            "line": index + 1,
            "protected": True,
        }
        rows.append(row)
        byte_offset += len(line.encode("utf-8")) + (index < len(lines) - 1)
        char_offset += len(line) + 1
        if frontmatter:
            if index > 0 and line in ("---", "..."):
                frontmatter = False
            continue
        if fence is not None:
            if re.fullmatch(
                r"[ \t]*" + re.escape(fence[0]) + "{" + str(fence[1]) + r",}[ \t]*",
                line,
            ):
                fence = None
            continue
        if strict_format and not comment:
            if html_end:
                if html_end in line.lower():
                    html_end = None
                continue
            if html_block:
                if not line.strip():
                    html_block = False
                continue
            raw = re.match(
                r"^ {0,3}<(pre|script|style|textarea)(?:\s|>|$)", line, re.IGNORECASE
            )
            if raw:
                ending = "</" + raw[1].lower() + ">"
                if ending not in line.lower():
                    html_end = ending
                continue
            if re.match(
                r"^ {0,3}</?(?:div|table|details|summary|section|article|blockquote|ul|ol|li|p|h[1-6]|figure|aside|dl|dt|dd)(?:\s|>|/)",
                line,
                re.IGNORECASE,
            ):
                html_block = True
                continue
        prefix = line[: len(line) - len(line.lstrip(" \t"))]
        indent = len(prefix.expandtabs(4))
        if not comment and indent <= 3 and line.strip() in (BEGIN, END):
            markers.append((index, line.strip()))
            continue
        # Distinguish a nested list from an indented code block within a list.
        # Check before masking comments so literal <!-- inside code stays code.
        parents = list_stack.copy()
        if LIST.match(line):
            while parents and indent <= parents[-1][0]:
                parents.pop()
        else:
            while parents and indent < parents[-1][1]:
                parents.pop()
        if (
            not comment
            and indent >= 4
            and (not parents or indent >= parents[-1][1] + 4)
        ):
            continue
        visible, comment = _mask_comments(line, comment)
        opening = FENCE.match(visible)
        if opening and (indent <= 3 or parents):
            marker, info = opening.groups()
            if marker[0] != "`" or "`" not in info:
                list_stack = parents
                fence = (marker[0], len(marker))
                continue
        item = LIST.match(visible)
        if visible.strip():
            list_stack = parents
            if item:
                list_stack.append((indent, len(visible[: item.end()].expandtabs(4))))
        row["visible"] = visible
        row["protected"] = comment or visible != line
    if frontmatter:
        raise IncompleteMarkdown(
            "YAML front matter is not closed; add its closing --- first."
        )
    if strict_format and fence is not None:
        raise IncompleteMarkdown(
            "Close the fenced code block before formatting Markdown."
        )
    block = None
    if markers:
        if len(markers) == 2 and markers[0][1] == BEGIN and markers[1][1] == END:
            first, last = markers[0][0], markers[1][0]
            block = (rows[first]["start"], rows[last]["end"])
            for row in rows[first : last + 1]:
                row["visible"] = ""
        elif strict_toc:
            raise PluginError(
                "Contents markers must contain exactly one matching start/end pair."
            )
    return text, rows, block


def _plain_heading(title):
    # Bounded patterns avoid quadratic scans on malformed, very long headings.
    title = re.sub(r"\[([^]\n]{0,512})\]\([^\n)]{0,2048}\)", r"\1", title)
    title = re.sub(r"</?[A-Za-z][^>\n]{0,512}>", "", title)
    return html.unescape(title.replace("`", "").replace("*", "")).strip()


def _headings(rows):
    headings = []
    for index, row in enumerate(rows):
        line = row["visible"]
        atx = ATX.match(line)
        if atx:
            level = len(atx[1])
            title = re.sub(r"[ \t]+#+[ \t]*$", "", atx[2] or "").strip()
            if re.fullmatch(r"#+", title):
                title = ""
        elif (
            line.strip()
            and not LIST.match(line)
            and len(line) - len(line.lstrip(" ")) <= 3
            and index + 1 < len(rows)
            and SETEXT.fullmatch(rows[index + 1]["visible"])
            and (index == 0 or not rows[index - 1]["visible"].strip())
        ):
            level = 1 if rows[index + 1]["visible"].lstrip().startswith("=") else 2
            title = line.strip()
        else:
            continue
        title = _plain_heading(title)
        if title:
            if len(title) > 500:
                raise PluginError(
                    "A heading exceeds 500 characters; shorten it before updating contents."
                )
            headings.append((index, level, title))
    return headings


def _slug(title, used, suffixes):
    base = "".join(
        c
        for c in title.lower()
        if unicodedata.category(c)[0] in "LNM" or c in "_-" or c.isspace()
    )
    base = re.sub(r"\s", "-", base) or "section"
    suffix = suffixes.get(base, 0)
    slug = f"{base}-{suffix}" if suffix else base
    while slug in used:
        suffix += 1
        slug = f"{base}-{suffix}"
    used.add(slug)
    suffixes[base] = suffix + 1
    return slug


def update_toc(document):
    text, rows, block = _scan(document, strict_toc=True)
    headings = _headings(rows)
    used, suffixes, entries = set(), {}, []
    for _, level, title in headings:
        slug = _slug(title, used, suffixes)
        if level == 1:
            continue
        label = (
            html.escape(title, quote=False)
            .replace("\\", "\\\\")
            .replace("[", "\\[")
            .replace("]", "\\]")
        )
        entries.append("  " * (level - 2) + f"- [{label}](#{slug})")
    if not entries and not block:
        return {
            "message": "No level 2–6 headings found. Add ## headings, then update contents."
        }
    if len(entries) > 500:
        raise PluginError("A contents block supports up to 500 headings.")
    contents = BEGIN + "\n" + ("\n".join(entries) + "\n" if entries else "") + END
    if block:
        start, end = block
        replacement = contents
    else:
        title = next((index for index, level, _ in headings if level == 1), None)
        if title is not None:
            # Include a Setext title's underline when choosing the insertion point.
            last = title + (0 if ATX.match(rows[title]["visible"]) else 1)
            start = end = rows[last]["end"]
            suffix = (
                ""
                if text[end:].startswith("\n\n")
                else "\n"
                if text[end:].startswith("\n")
                else "\n\n"
            )
            replacement = "\n\n" + contents + suffix
        else:
            # Keep YAML metadata first even when a document has no H1 title.
            start = end = 0
            if rows and rows[0]["text"].lstrip("\ufeff") == "---":
                closing = next(
                    i for i in range(1, len(rows)) if rows[i]["text"] in ("---", "...")
                )
                start = end = rows[closing]["end"]
            suffix = (
                ""
                if text[end:].startswith("\n\n")
                else "\n"
                if text[end:].startswith("\n")
                else "\n\n"
            )
            replacement = ("\n\n" if start else "") + contents + suffix
    if text[start:end] == replacement:
        return {"message": "Table of contents is already up to date."}
    return {
        "title": "Update Markdown table of contents",
        "edits": [
            {
                "start": len(text[:start].encode("utf-8")),
                "end": len(text[:end].encode("utf-8")),
                "text": replacement,
            }
        ],
    }


def _tasks(rows):
    return [(row, match) for row in rows if (match := TASK.match(row["visible"]))]


def task_summary(document):
    _, rows, _ = _scan(document)
    tasks = _tasks(rows)
    if not tasks:
        return {
            "message": "No Markdown tasks found. Use - [ ] for pending tasks and - [x] for completed tasks."
        }
    pending = [(row, match) for row, match in tasks if match[1] == " "]
    done = len(tasks) - len(pending)
    body = [
        f"Completed: {done}/{len(tasks)} ({done * 100 // len(tasks)}%)",
        f"Remaining: {len(pending)}",
        "",
        "Pending tasks:",
    ]
    for row, match in pending[:100]:
        label = (match[2] or "").strip() or "(untitled task)"
        body.append(
            f"Line {row['line']}: {label[:240]}" + ("…" if len(label) > 240 else "")
        )
    if len(pending) > 100:
        body.append(f"… {len(pending) - 100} more pending tasks")
    if not pending:
        body.append("All tasks are complete.")
    return {"title": "Markdown task progress", "body": "\n".join(body)}


def toggle_tasks(document):
    _, rows, _ = _scan(document)
    ranges = []
    if len(document.get("selections", [])) > 256:
        raise PluginError(
            "Toggle Selected Tasks supports up to 256 selections at a time."
        )
    for selection in document.get("selections", []):
        _, start, end = selected_text(document, selection)
        ranges.append((start, end))
    edits = []
    for row, match in _tasks(rows):
        start = row["byte"]
        end = start + len(row["text"].encode("utf-8"))
        if any(
            (start <= lo <= end if lo == hi else start < hi and end > lo)
            for lo, hi in ranges
        ):
            flag = start + len(row["text"][: match.start(1)].encode("utf-8"))
            edits.append(
                {
                    "start": flag,
                    "end": flag + 1,
                    "text": "x" if match[1] == " " else " ",
                }
            )
    if not edits:
        return {
            "message": "Place the cursor on a task, or select task lines, then run Toggle Selected Tasks."
        }
    return {"title": f"Toggle {len(edits)} Markdown tasks", "edits": edits}


def format_preview(document):
    try:
        source, rows, _ = _scan(document, strict_format=True)
    except IncompleteMarkdown as exc:
        return {
            "title": "Markdown preview",
            "preview": {"kind": "markdown", "text": document["text"]},
            "edits": [],
            "message": str(exc),
        }
    formatted = format_document(source, rows)
    return {
        "title": "Markdown formatting",
        "preview": {"kind": "markdown", "text": formatted},
        "edits": (
            [{"start": 0, "end": len(source.encode("utf-8")), "text": formatted}]
            if source != formatted
            else []
        ),
    }


if __name__ == "__main__":
    Plugin(
        {
            "format-preview": format_preview,
            "update-toc": update_toc,
            "task-summary": task_summary,
            "toggle-tasks": toggle_tasks,
        }
    ).run()
