import json
import logging
import os
import subprocess

import magic

log = logging.getLogger("engram-worker")

# OCR config — read once at import. Disable by setting OCR_ENABLED=false.
_OCR_ENABLED = os.environ.get("OCR_ENABLED", "true").lower() != "false"
_OCR_LANG = os.environ.get("OCR_LANG", "eng")
_OCR_TIMEOUT_SECS = int(os.environ.get("OCR_TIMEOUT_SECS", "30"))
_OCR_MAX_BYTES = int(os.environ.get("OCR_MAX_BYTES", str(50 * 1024 * 1024)))
_OCR_PDF_MAX_PAGES = int(os.environ.get("OCR_PDF_MAX_PAGES", "200"))


def detect_mime(filepath: str) -> str:
    return magic.from_file(filepath, mime=True)


def ocr_image(filepath: str) -> str:
    """Run Tesseract OCR on an image path; return text (may be empty).

    Swallows all failures (binary missing, timeout, image unreadable) and
    returns "" — OCR is best-effort and must never block ingestion.
    """
    if not _OCR_ENABLED:
        return ""
    try:
        if os.path.getsize(filepath) > _OCR_MAX_BYTES:
            log.info("ocr: skipping, file too large: %s", filepath)
            return ""
    except OSError:
        return ""
    try:
        # Invoke tesseract via subprocess directly (not pytesseract) so we get
        # a hard timeout — pytesseract can wedge on pathological inputs.
        result = subprocess.run(
            ["tesseract", filepath, "-", "-l", _OCR_LANG],
            capture_output=True,
            text=True,
            timeout=_OCR_TIMEOUT_SECS,
        )
        if result.returncode != 0:
            log.warning("ocr: tesseract exited %d: %s",
                        result.returncode, result.stderr[:200])
            return ""
        return result.stdout.strip()
    except (subprocess.TimeoutExpired, FileNotFoundError, OSError) as e:
        log.warning("ocr: failed on %s: %s", filepath, e)
        return ""


def extract_pdf(filepath: str) -> dict:
    """Extract text from a PDF. For pages that yield little or no text
    (typical of scanned documents), rasterize and OCR them."""
    import pymupdf

    doc = pymupdf.open(filepath)
    page_count = doc.page_count
    text_parts = []
    ocr_pages = 0

    for idx, page in enumerate(doc):
        page_text = page.get_text()
        if len(page_text.strip()) < 50 and _OCR_ENABLED and idx < _OCR_PDF_MAX_PAGES:
            # Likely a scanned page — rasterize and OCR.
            try:
                pix = page.get_pixmap(dpi=200)
                png_bytes = pix.tobytes("png")
                ocr_text = _ocr_png_bytes(png_bytes)
                if ocr_text:
                    page_text = ocr_text
                    ocr_pages += 1
            except Exception as e:
                log.warning("ocr: pdf page %d failed: %s", idx, e)
        text_parts.append(page_text)

    doc.close()
    text = "\n".join(text_parts).strip()
    if len(text) > 100_000:
        text = text[:100_000]

    if ocr_pages:
        log.info("ocr: used on %d/%d pages of %s", ocr_pages, page_count, filepath)

    return {"text": text, "page_count": page_count}


def _ocr_png_bytes(png_bytes: bytes) -> str:
    """OCR in-memory PNG bytes; returns text or ""."""
    if not _OCR_ENABLED:
        return ""
    try:
        result = subprocess.run(
            ["tesseract", "stdin", "-", "-l", _OCR_LANG],
            input=png_bytes,
            capture_output=True,
            timeout=_OCR_TIMEOUT_SECS,
        )
        if result.returncode != 0:
            return ""
        return result.stdout.decode("utf-8", errors="replace").strip()
    except (subprocess.TimeoutExpired, FileNotFoundError, OSError):
        return ""


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
