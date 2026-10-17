"""Conservative Markdown source formatting; preserve code and paragraph breaks."""

import re
import unicodedata

HEADING = re.compile(r"^( {0,3})(#{1,6})[ \t]+(.*)$")
BULLET = re.compile(r"^( {0,3})[+*]([ \t]+)(\S.*)$")
TASK = re.compile(r"^(\s*(?:[-+*]|[0-9]+[.)])[ \t]+)\[X\](?=[ \t]|$)")
RULE = re.compile(
    r"^ {0,3}(?:\*[ \t]*){3,}$|^ {0,3}(?:-[ \t]*){3,}$|^ {0,3}(?:_[ \t]*){3,}$"
)


def _cells(line):
    if len(line) - len(line.lstrip(" ")) > 3 or "|" not in line:
        return None
    parts = re.split(r"(?<!\\)\|", line.strip())
    if parts[0] == "":
        parts.pop(0)
    if parts and parts[-1] == "":
        parts.pop()
    return [part.strip() for part in parts]


def _width(text):
    return sum(
        0
        if unicodedata.combining(c) or c == "\u200d"
        else 2
        if unicodedata.east_asian_width(c) in "WF"
        else 1
        for c in text
    )


def _tables(rows):
    replacements = {}
    index = 0
    while index + 1 < len(rows):
        heading, delimiter = rows[index], rows[index + 1]
        cells = _cells(heading["text"])
        dividers = _cells(delimiter["text"])
        if (
            heading["protected"]
            or delimiter["protected"]
            or not cells
            or not dividers
            or len(cells) != len(dividers)
            or len(cells) > 32
            or not all(re.fullmatch(r":?-{3,}:?", cell) for cell in dividers)
        ):
            index += 1
            continue
        table = [cells]
        prefix = heading["text"][
            : len(heading["text"]) - len(heading["text"].lstrip(" "))
        ]
        end = index + 2
        while end < len(rows) and not rows[end]["protected"]:
            data = _cells(rows[end]["text"])
            if not data or len(data) != len(cells):
                break
            table.append(data)
            end += 1
        # Bound alignment cost and width; large tables are preserved verbatim.
        widths = [
            max(3, max(_width(row[column]) for row in table), len(dividers[column]))
            for column in range(len(cells))
        ]
        if max(widths) > 120 or len(table) > 1000:
            index = end
            continue
        for offset, data in enumerate(table):
            target = index if offset == 0 else index + offset + 1
            replacements[target] = (
                prefix
                + "| "
                + " | ".join(
                    cell + " " * (width - _width(cell))
                    for cell, width in zip(data, widths)
                )
                + " |"
            )
        separators = []
        for cell, width in zip(dividers, widths):
            left, right = cell.startswith(":"), cell.endswith(":")
            separators.append(
                (":" if left else "")
                + "-" * (width - left - right)
                + (":" if right else "")
            )
        replacements[index + 1] = prefix + "| " + " | ".join(separators) + " |"
        index = end
    return replacements


def format_document(source, rows):
    if not source:
        return ""
    out = []
    tables = _tables(rows)
    previous_heading = False
    # Preserve line count inside protected blocks, including all code spacing.
    for index, row in enumerate(rows):
        line = row["text"]
        if index == len(rows) - 1 and line == "":
            continue
        if row["protected"]:
            out.append(line)
            previous_heading = False
            continue
        if index in tables:
            out.append(tables[index])
            previous_heading = False
            continue
        heading = HEADING.match(line)
        if not line.strip():
            if out and out[-1] != "":
                out.append("")
            previous_heading = False
            continue
        if heading:
            # Closing hashes are optional syntax and never part of the heading.
            title = re.sub(r"[ \t]+#+[ \t]*$", "", heading[3]).strip()
            line = heading[1] + heading[2] + " " + title
            if out and out[-1] != "":
                out.append("")
        else:
            if previous_heading and out and out[-1] != "":
                out.append("")
            if not RULE.fullmatch(line):
                line = BULLET.sub(r"\1-\2\3", line)
            line = TASK.sub(r"\1[x]", line)
            # Two trailing spaces mean a hard break; preserve that semantics.
            trimmed = line.rstrip(" \t")
            line = trimmed + ("  " if line.endswith("  ") else "")
        out.append(line)
        previous_heading = bool(heading)
    while out and out[-1] == "":
        out.pop()
    return "\n".join(out) + ("\n" if out else "")
