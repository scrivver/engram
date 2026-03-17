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

    with Image.open(filepath) as img:
        width, height = img.size
    return {"width": width, "height": height}


def extract_text(filepath: str) -> dict:
    with open(filepath, errors="replace") as f:
        text = f.read(100_000)
    return {"text": text}
