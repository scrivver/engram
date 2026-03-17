import json
import subprocess

import magic


def detect_mime(filepath: str) -> str:
    return magic.from_file(filepath, mime=True)


def extract_pdf(filepath: str) -> dict:
    import pymupdf

    doc = pymupdf.open(filepath)
    text_parts = []
    for page in doc:
        text_parts.append(page.get_text())
    page_count = doc.page_count
    doc.close()

    text = "\n".join(text_parts).strip()
    if len(text) > 100_000:
        text = text[:100_000]

    return {"text": text, "page_count": page_count}


def extract_image(filepath: str) -> dict:
    from PIL import Image
    from PIL.ExifTags import GPSTAGS

    result = {}
    with Image.open(filepath) as img:
        result["width"], result["height"] = img.size

        exif_data = img.getexif()
        if exif_data:
            # GPS info is in IFD 0x8825
            gps_ifd = exif_data.get_ifd(0x8825)
            if gps_ifd:
                gps = {}
                for tag_id, value in gps_ifd.items():
                    tag_name = GPSTAGS.get(tag_id, tag_id)
                    gps[tag_name] = value

                lat = _convert_gps_coord(
                    gps.get("GPSLatitude"), gps.get("GPSLatitudeRef")
                )
                lon = _convert_gps_coord(
                    gps.get("GPSLongitude"), gps.get("GPSLongitudeRef")
                )
                if lat is not None and lon is not None:
                    result["gps"] = {"lat": lat, "lon": lon}

    return result


def _convert_gps_coord(coord, ref) -> float | None:
    """Convert EXIF GPS coordinate (degrees, minutes, seconds) to decimal."""
    if coord is None or ref is None:
        return None
    try:
        degrees = float(coord[0])
        minutes = float(coord[1])
        seconds = float(coord[2])
        decimal = degrees + minutes / 60 + seconds / 3600
        if ref in ("S", "W"):
            decimal = -decimal
        return decimal
    except (TypeError, IndexError, ValueError):
        return None


def extract_text(filepath: str) -> dict:
    with open(filepath, errors="replace") as f:
        text = f.read(100_000)
    return {"text": text}


def extract_media(filepath: str) -> dict:
    """Extract audio/video metadata using ffprobe."""
    try:
        result = subprocess.run(
            [
                "ffprobe",
                "-v", "quiet",
                "-print_format", "json",
                "-show_format",
                "-show_streams",
                filepath,
            ],
            capture_output=True,
            text=True,
            timeout=30,
        )
        if result.returncode != 0:
            return {}

        data = json.loads(result.stdout)
        info = {}

        # Duration
        fmt = data.get("format", {})
        if "duration" in fmt:
            info["duration"] = float(fmt["duration"])

        # Embedded tags (title, artist, album, genre, etc.)
        tags = fmt.get("tags", {})
        # Normalize tag keys to lowercase
        tags = {k.lower(): v for k, v in tags.items()}
        for key in ("title", "artist", "album", "genre"):
            if key in tags:
                info[key] = tags[key]

        # Video stream info
        for stream in data.get("streams", []):
            if stream.get("codec_type") == "video":
                width = stream.get("width")
                height = stream.get("height")
                if width and height:
                    info["width"] = width
                    info["height"] = height
                break

        return info

    except (subprocess.TimeoutExpired, json.JSONDecodeError, FileNotFoundError):
        return {}
