"""Include the installable Markdown plugin's command tests in make smoke/CI."""

import sys
from pathlib import Path
from unittest import TestLoader
from unittest.mock import patch


def load_tests(loader, tests, pattern):
    root = Path(__file__).resolve().parent.parent / "examples/markdown-tools"
    with patch.object(sys, "dont_write_bytecode", True):
        return TestLoader().discover(
            str(root), pattern="test_markdown_commands.py", top_level_dir=str(root)
        )
