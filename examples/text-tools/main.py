from sunbook_plugin import Plugin, PluginError, selected_text


def word_count(document):
    if document is None:
        raise PluginError("Open a document first")
    text = document["text"]
    return {"title": "Document summary",
            "body": f"Words: {len(text.split())}\nLines: {text.count(chr(10)) + 1}\nCharacters: {len(text)}\nUTF-8 bytes: {len(text.encode('utf-8'))}"}


def uppercase_selection(document):
    if document is None:
        raise PluginError("Open a document first")
    edits = []
    for selection in document["selections"]:
        text, start, end = selected_text(document, selection)
        if start != end:
            edits.append({"start": start, "end": end, "text": text.upper()})
    if not edits:
        return {"message": "Select text, then run Uppercase Selection again."}
    return {"title": "Uppercase selection", "edits": edits}


if __name__ == "__main__":
    Plugin({"word-count": word_count, "uppercase-selection": uppercase_selection}).run()
