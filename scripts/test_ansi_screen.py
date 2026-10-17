import unittest
from ansi_screen import Screen


class ScreenTests(unittest.TestCase):
    def test_differential_frame_preserves_existing_letters(self):
        screen = Screen()
        screen.feed(b"\x1b[1;1HHELLO\x1b[?2026l")
        screen.feed(b"\x1b[1;1HJ\x1b[1;5HY\x1b[?2026l")
        self.assertEqual(screen.text, "JELLY")
        self.assertEqual(screen.generation, 2)

    def test_split_utf8_sequences_wide_cells_and_erase(self):
        screen = Screen()
        data = "\x1b[2;1H中👩‍💻x\x1b[2;5HY\x1b[?2026l".encode()
        for byte in data:
            screen.feed(bytes([byte]))
        self.assertEqual(screen.text, "中👩Y")
        screen.feed(b"\x1b[2;1H\x1b[2Knew\x1b[?2026l")
        self.assertEqual(screen.text, "new")

    def test_frames_and_terminals_without_synchronized_output(self):
        screen = Screen()
        screen.feed(b"legacy")
        self.assertEqual(screen.text, "legacy")
        screen.feed(b"\x1b[?2026h\rmodern")
        self.assertEqual(screen.text, "legacy")
        screen.feed(b"\x1b[?2026l")
        self.assertEqual(screen.text, "modern")

    def test_cursor_moves_clear_and_osc(self):
        screen = Screen()
        screen.feed(b"old\x1b[2J\x1b]0;ignore\x07\x1b[1;1Habc\x08X\x1b[?2026l")
        self.assertEqual(screen.text, "abX")


if __name__ == "__main__":
    unittest.main()
