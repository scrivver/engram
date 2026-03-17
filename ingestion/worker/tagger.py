import os
import re
from datetime import datetime

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

MONTH_NAMES = [
    "january", "february", "march", "april", "may", "june",
    "july", "august", "september", "october", "november", "december",
]

RESOLUTION_LABELS = {
    720: "720p",
    1080: "1080p",
    1440: "1440p",
    2160: "4k",
    4320: "8k",
}


def _slugify(text: str) -> str:
    """Convert text to a lowercase slug suitable for tags."""
    text = text.lower().strip()
    text = re.sub(r"[^a-z0-9]+", "-", text)
    return text.strip("-")


def generate_tags(
    filename: str,
    mime_type: str,
    mtime: str | None = None,
    gps: dict | None = None,
    media_info: dict | None = None,
) -> list[str]:
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

    # Time-based tags from mtime
    if mtime:
        try:
            dt = datetime.fromisoformat(mtime.replace("Z", "+00:00"))
            tags.add(str(dt.year))
            tags.add(f"{dt.year}-{dt.month:02d}")
            tags.add(MONTH_NAMES[dt.month - 1])
        except (ValueError, IndexError):
            pass

    # Location tags from GPS coordinates
    if gps and "lat" in gps and "lon" in gps:
        try:
            import reverse_geocode

            result = reverse_geocode.get((gps["lat"], gps["lon"]))
            if result:
                city = result.get("city")
                country = result.get("country")
                if city:
                    tags.add(_slugify(city))
                if country:
                    tags.add(_slugify(country))
        except Exception:
            pass

    # Audio/video tags from ffprobe metadata
    if media_info:
        # Resolution tags
        height = media_info.get("height")
        if height:
            for threshold, label in sorted(RESOLUTION_LABELS.items()):
                if height <= threshold:
                    tags.add(label)
                    break
            else:
                if height > 4320:
                    tags.add("8k")

        # Duration tags
        duration = media_info.get("duration")
        if duration is not None:
            if duration < 60:
                tags.add("short")
            elif duration < 600:
                tags.add("medium")
            else:
                tags.add("long")

        # Genre tag
        genre = media_info.get("genre")
        if genre:
            tags.add(_slugify(genre))

        # Artist tag
        artist = media_info.get("artist")
        if artist:
            tags.add(f"artist:{_slugify(artist)}")

    return sorted(tags)
