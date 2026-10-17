import importlib.util
import unittest
from pathlib import Path

from sunbook_plugin import Plugin, PluginError

spec = importlib.util.spec_from_file_location(
    "markdown_tools_main", Path(__file__).with_name("main.py")
)
tools = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tools)


def document(text, selections=None):
    return {
        "path": "notes.md",
        "version": 1,
        "text": text,
        "selections": selections
        if selections is not None
        else [{"anchor": 0, "head": 0}],
    }


def apply(text, result):
    raw = text.encode("utf-8")
    for edit in sorted(
        result.get("edits", []), key=lambda edit: edit["start"], reverse=True
    ):
        raw = raw[: edit["start"]] + edit["text"].encode("utf-8") + raw[edit["end"] :]
    return raw.decode("utf-8")


class MarkdownCommandsTests(unittest.TestCase):
    def test_format_aligns_tables_without_modifying_escaped_pipes_or_code(self):
        source = "| 名称 | 描述 |\n|:---|---:|\n| 中文 | a\\|b |\n| x | value |\n\n```md\n|a|b|\n|---|---|\n```\n"
        result = tools.format_preview(document(source))["preview"]["text"]
        self.assertIn("| 中文 | a\\|b  |", result)
        self.assertIn("| :--- | ----: |", result)
        self.assertIn("```md\n|a|b|\n|---|---|\n```", result)
        self.assertEqual(tools.format_preview(document(result))["edits"], [])

    def test_format_and_split_preview_preserves_code_and_is_idempotent(self):
        source = "#   项目 👩‍💻   ###\n正文  \n换行\n\n\n##    开始\n+ [X] 任务\n\n```python\n#    保持代码  \n\n\nprint('hello')  \n```\n"
        result = tools.format_preview(document(source))
        candidate = result["preview"]["text"]
        self.assertEqual(result["preview"]["kind"], "markdown")
        self.assertIn("# 项目 👩‍💻\n\n正文  \n换行\n\n## 开始\n\n- [x] 任务", candidate)
        self.assertIn(
            "```python\n#    保持代码  \n\n\nprint('hello')  \n```", candidate
        )
        self.assertEqual(apply(source, result), candidate)
        again = tools.format_preview(document(candidate))
        self.assertEqual(again["preview"]["text"], candidate)
        self.assertEqual(again["edits"], [])

    def test_format_preserves_frontmatter_html_rules_and_indented_code(self):
        source = "---\ntitle:  Notes  \n---\n\n<pre>\n#   literal  \n* literal\n</pre>\n\n* * *\n\n    ## code  \n\n##   Actual\ntext\n"
        formatted = tools.format_preview(document(source))["preview"]["text"]
        self.assertTrue(formatted.startswith("---\ntitle:  Notes  \n---"))
        self.assertIn("<pre>\n#   literal  \n* literal\n</pre>", formatted)
        self.assertIn("\n* * *\n", formatted)
        self.assertIn("    ## code  ", formatted)
        self.assertIn("## Actual\n\ntext\n", formatted)
        incomplete = "# Notes\n```\nunclosed"
        result = tools.format_preview(document(incomplete))
        self.assertEqual(result["preview"]["text"], incomplete)
        self.assertEqual(result["edits"], [])
        self.assertIn("Close the fenced", result["message"])
        result = tools.format_preview(document("---\ntitle: incomplete"))
        self.assertEqual(result["edits"], [])
        self.assertIn("front matter", result["message"])
        self.assertEqual(tools.format_preview(document(""))["preview"]["text"], "")
        fenced_html = "```html\n<pre>\n#  literal  \n```\n\n##   Real\n"
        result = tools.format_preview(document(fenced_html))["preview"]["text"]
        self.assertIn("```html\n<pre>\n#  literal  \n```", result)
        self.assertIn("## Real", result)

    def test_toc_unicode_duplicates_and_idempotency(self):
        text = "# 笔记 👩‍💻\n\n## 开始\n\n### Café e\u0301\n\n## 开始\n\n## 开始-1\n"
        updated = apply(text, tools.update_toc(document(text)))
        self.assertTrue(updated.startswith("# 笔记 👩‍💻\n\n" + tools.BEGIN))
        self.assertIn("- [开始](#开始)", updated)
        self.assertIn("  - [Café e\u0301](#café-e\u0301)", updated)
        self.assertIn("- [开始](#开始-1)", updated)
        self.assertIn("- [开始-1](#开始-1-1)", updated)
        self.assertIn(tools.END + "\n\n## 开始", updated)
        self.assertNotIn("edits", tools.update_toc(document(updated)))

    def test_update_only_managed_block_and_preserve_surrounding_text(self):
        text = (
            "# 标题\n\nIntro stays.\n\n"
            + tools.BEGIN
            + "\nold\n"
            + tools.END
            + "\n\n## New\nTail stays.\n"
        )
        result = tools.update_toc(document(text))
        self.assertEqual(len(result["edits"]), 1)
        updated = apply(text, result)
        self.assertTrue(updated.startswith("# 标题\n\nIntro stays.\n\n"))
        self.assertTrue(updated.endswith("\n\n## New\nTail stays.\n"))
        self.assertNotIn("\nold\n", updated)
        no_sections = "# Notes\n\n" + tools.BEGIN + "\n- [Old](#old)\n" + tools.END
        empty = apply(no_sections, tools.update_toc(document(no_sections)))
        self.assertNotIn("[Old]", empty)
        self.assertNotIn("edits", tools.update_toc(document(empty)))

    def test_frontmatter_setext_and_inline_titles(self):
        text = "---\ntitle: Notes\n---\n\nNotes\n=====\n\nGetting `started`\n-----------------\n\n## [API](https://example.invalid) &amp; <b>Tools</b> ###\n"
        updated = apply(text, tools.update_toc(document(text)))
        self.assertTrue(
            updated.startswith("---\ntitle: Notes\n---\n\nNotes\n=====\n\n")
        )
        self.assertIn("[Getting started](#getting-started)", updated)
        self.assertIn("[API &amp; Tools](#api--tools)", updated)
        self.assertNotIn("edits", tools.update_toc(document(updated)))
        no_title = "---\ntitle: Notes\n...\n\n## Section\n"
        self.assertTrue(
            apply(no_title, tools.update_toc(document(no_title))).startswith(
                "---\ntitle: Notes\n...\n\n" + tools.BEGIN
            )
        )

    def test_fences_comments_and_indented_code_are_ignored(self):
        text = "# Notes\n\n```md\n## Code\n- [ ] fake\n<!-- sunbook:toc -->\n```\n\n~~~~\n## More code\n- [ ] fake\n~~~\n- [ ] still fenced\n~~~~\n\n<!--\n## Comment\n- [ ] fake\n-->\n\n    ## Indented\n    - [ ] fake\n\n\t- [ ] tab code\n\n## Real\n- [ ] pending\n- [X] done\n"
        result = tools.update_toc(document(text))
        self.assertEqual(result["edits"][0]["text"].count("](#"), 1)
        self.assertIn("[Real](#real)", result["edits"][0]["text"])
        summary = tools.task_summary(document(text))
        self.assertIn("Completed: 1/2 (50%)", summary["body"])
        self.assertNotIn("fake", summary["body"])

    def test_task_counts_nested_ordered_empty_and_capped_output(self):
        text = "- [x] done\n  - [ ] 中文 👩‍💻\n    - [X] nested done\n1. [ ] ordered\n2) [ ]\n"
        body = tools.task_summary(document(text))["body"]
        self.assertIn("Completed: 2/5 (40%)", body)
        self.assertIn("Remaining: 3", body)
        self.assertIn("中文 👩‍💻", body)
        self.assertIn("(untitled task)", body)
        body = tools.task_summary(document("- [ ] pending\n" * 105))["body"]
        self.assertEqual(body.count("Line "), 100)
        self.assertIn("5 more", body)
        self.assertIn(
            "All tasks are complete.",
            tools.task_summary(document("- [x] done"))["body"],
        )

    def test_indented_code_inside_lists_and_literal_comments(self):
        text = "- parent\n\n      - [ ] indented code\n      <!-- literal code comment\n\n    - [ ] real nested task\n        - [x] deeper task\n\n## Visible heading\n- [ ] real root task\n"
        summary = tools.task_summary(document(text))["body"]
        self.assertIn("Completed: 1/3 (33%)", summary)
        self.assertNotIn("indented code", summary)
        self.assertIn(
            "[Visible heading](#visible-heading)",
            tools.update_toc(document(text))["edits"][0]["text"],
        )

    def test_toggle_unicode_reverse_and_overlapping_selections(self):
        text = "# 计划 👩‍💻 e\u0301\n\n- [ ] 编写\n- [X] 测试\n"
        length = len(text.encode("utf-8"))
        doc = document(
            text, [{"anchor": length, "head": 0}, {"anchor": 0, "head": length}]
        )
        result = tools.toggle_tasks(doc)
        self.assertEqual(len(result["edits"]), 2)
        self.assertEqual(
            apply(text, result),
            text.replace("[ ] 编写", "[x] 编写").replace("[X] 测试", "[ ] 测试"),
        )
        self.assertTrue(
            all(edit["end"] - edit["start"] == 1 for edit in result["edits"])
        )

    def test_caret_and_selection_ending_at_next_line_start(self):
        text = "- [ ] one\n- [x] two\n"
        next_line = len("- [ ] one\n")
        first = tools.toggle_tasks(document(text, [{"anchor": 0, "head": next_line}]))
        self.assertEqual(apply(text, first), "- [x] one\n- [x] two\n")
        caret = tools.toggle_tasks(
            document(text, [{"anchor": next_line, "head": next_line}])
        )
        self.assertEqual(apply(text, caret), "- [ ] one\n- [ ] two\n")

    def test_no_document_empty_inputs_and_no_selected_tasks(self):
        for command in (tools.update_toc, tools.task_summary, tools.toggle_tasks):
            with self.assertRaises(PluginError):
                command(None)
            self.assertNotIn("edits", command(document("")))
        self.assertNotIn("edits", tools.toggle_tasks(document("Text\n- [ ] task")))
        self.assertNotIn("edits", tools.update_toc(document("# Title only\n## ###\n")))

    def test_malformed_markers_and_frontmatter_fail_without_edits(self):
        for text in (
            tools.BEGIN + "\n## A",
            tools.END + "\n" + tools.BEGIN,
            tools.BEGIN + "\n" + tools.END + "\n" + tools.BEGIN,
            "---\ntitle: unfinished\n## Heading",
        ):
            with self.subTest(text=text), self.assertRaises(PluginError):
                tools.update_toc(document(text))
        summary = tools.task_summary(document(tools.BEGIN + "\n- [ ] still a task"))
        self.assertIn("Remaining: 1", summary["body"])

    def test_limits_and_bad_byte_offsets_are_recoverable(self):
        with self.assertRaises(PluginError):
            tools.task_summary(document("x" * (1024 * 1024 + 1)))
        with self.assertRaises(PluginError):
            tools.update_toc(document("## Heading\n" * 501))
        with self.assertRaises(PluginError):
            tools.toggle_tasks(document("- [ ] one", [{"anchor": 0, "head": 0}] * 257))
        with self.assertRaises(UnicodeDecodeError):
            tools.toggle_tasks(document("中\n- [ ] one", [{"anchor": 1, "head": 3}]))
        plugin = Plugin({"update-toc": tools.update_toc})
        plugin.dispatch("initialize", {"protocolVersion": 1})
        with self.assertRaises(PluginError):
            plugin.dispatch("execute", {"command": "update-toc"})
        self.assertEqual(plugin.dispatch("ping", {}), {})


if __name__ == "__main__":
    unittest.main()
