import os

MIME_TAGS = {
    "application/pdf": "pdf",
    "image/": "image",
    "video/": "video",
    "audio/": "audio",
    "text/": "text",
}

EXT_TAGS = {
    ".py": "code",
    ".go": "code",
    ".js": "code",
    ".ts": "code",
    ".rs": "code",
    ".c": "code",
    ".h": "code",
    ".java": "code",
    ".rb": "code",
    ".sh": "code",
    ".xlsx": "spreadsheet",
    ".xls": "spreadsheet",
    ".csv": "spreadsheet",
    ".doc": "document",
    ".docx": "document",
    ".odt": "document",
    ".ppt": "presentation",
    ".pptx": "presentation",
}

NAME_PATTERNS = ["invoice", "receipt", "resume", "report", "contract", "screenshot"]


def generate_tags(filename: str, mime_type: str) -> list[str]:
    tags = set()

    # Tag by MIME type
    for prefix, tag in MIME_TAGS.items():
        if mime_type == prefix or mime_type.startswith(prefix):
            tags.add(tag)
            break

    # Tag by extension
    ext = os.path.splitext(filename)[1].lower()
    if ext in EXT_TAGS:
        tags.add(EXT_TAGS[ext])

    # Tag by filename patterns
    lower_name = filename.lower()
    for pattern in NAME_PATTERNS:
        if pattern in lower_name:
            tags.add(pattern)

    return sorted(tags)
