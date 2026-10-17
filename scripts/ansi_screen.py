"""Small screen reader for tcell's cursor-addressed PTY output in smoke tests.

It handles the drawing sequences emitted by SunBook, not arbitrary terminal apps.
Assertions must inspect the reconstructed screen: a valid differential update can
omit letters that were already present at the same coordinates.
"""
import codecs
import unicodedata


class Screen:
    def __init__(self):
        self.decoder = codecs.getincrementaldecoder("utf-8")("replace")
        self.cells = {}
        self.x = self.y = 0
        self.state = "text"
        self.sequence = ""
        self.generation = 0
        self.text = ""
        self.join = False
        self.synchronized = False
        self.dirty = False

    def feed(self, data):
        for char in self.decoder.decode(data):
            if self.state == "osc":
                if char == "\a":
                    self.state = "text"
                elif char == "\x1b":
                    self.state = "osc_escape"
                continue
            if self.state == "osc_escape":
                self.state = "text" if char == "\\" else "osc"
                continue
            if self.state == "escape":
                self.state = {"[": "csi", "]": "osc"}.get(char, "text")
                self.sequence = ""
                continue
            if self.state == "csi":
                if "@" <= char <= "~":
                    self.csi(self.sequence, char)
                    self.state = "text"
                else:
                    self.sequence += char
                continue
            if char == "\x1b":
                self.state = "escape"
            elif char == "\r":
                self.x = 0
            elif char == "\n":
                self.y += 1
            elif char == "\b":
                self.x = max(0, self.x - 1)
            elif char == "\t":
                self.x = (self.x // 8 + 1) * 8
            elif ord(char) >= 32 and char != "\x7f":
                if char == "\u200d":
                    self.join = True
                    continue
                if self.join:
                    self.join = False
                    continue
                if unicodedata.combining(char) or char in ("\ufe0e", "\ufe0f"):
                    continue
                width = 2 if unicodedata.east_asian_width(char) in ("W", "F") else 1
                self.dirty = True
                self.cells[self.y, self.x] = char
                if width == 2:
                    self.cells[self.y, self.x + 1] = ""
                self.x += width

        if self.dirty and not self.synchronized:
            self.commit()

    def commit(self):
        self.generation += 1
        self.text = self.render()
        self.dirty = False

    def csi(self, sequence, final):
        if sequence == "?2026":
            if final == "h":
                self.synchronized = True
            elif final == "l":
                self.synchronized = False
                self.commit()
            return
        if sequence.startswith(("?", ">", "=")):
            if sequence == "?1049" and final == "h":
                self.cells.clear()
            return
        if any(c not in "0123456789;" for c in sequence):
            return
        values = [int(n) if n else 0 for n in sequence.split(";")]
        n = values[0] or 1
        if final in ("H", "f"):
            self.y = max(0, values[0] - 1)
            self.x = max(0, (values[1] if len(values) > 1 else 1) - 1)
        elif final == "G":
            self.x = n - 1
        elif final == "d":
            self.y = n - 1
        elif final == "A":
            self.y = max(0, self.y - n)
        elif final == "B":
            self.y += n
        elif final == "C":
            self.x += n
        elif final == "D":
            self.x = max(0, self.x - n)
        elif final == "J":
            self.dirty = True
            mode = values[0]
            for y, x in list(self.cells):
                if mode == 2 or (mode == 0 and (y, x) >= (self.y, self.x)) or (mode == 1 and (y, x) <= (self.y, self.x)):
                    del self.cells[y, x]
        elif final in ("K", "X"):
            self.dirty = True
            mode = values[0]
            for y, x in list(self.cells):
                if y == self.y and ((final == "X" and self.x <= x < self.x + n) or
                                   (final == "K" and (mode == 2 or (mode == 0 and x >= self.x) or (mode == 1 and x <= self.x)))):
                    del self.cells[y, x]

    def render(self):
        rows = {}
        for (y, x), value in self.cells.items():
            rows.setdefault(y, {})[x] = value
        return "\n".join("".join(row.get(x, " ") for x in range(max(row) + 1))
                         for _, row in sorted(rows.items()))
