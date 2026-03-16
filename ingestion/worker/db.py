import os

import psycopg2


def _connect():
    return psycopg2.connect(host=os.environ.get("PGHOST", "/tmp"), dbname="engram")


def update_file_status(file_id: str, status: str):
    with _connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE files SET status = %s, updated_at = now() WHERE id = %s",
                (status, file_id),
            )
        conn.commit()


def update_file_metadata(
    file_id: str,
    mime_type: str,
    page_count: int | None,
    extracted_text: str | None,
    tags: list[str],
):
    with _connect() as conn:
        with conn.cursor() as cur:
            cur.execute(
                """UPDATE files
                   SET mime_type = %s, page_count = %s, extracted_text = %s,
                       status = 'ready', updated_at = now()
                   WHERE id = %s""",
                (mime_type, page_count, extracted_text, file_id),
            )

            for tag_name in tags:
                cur.execute(
                    "INSERT INTO tags (name) VALUES (%s) ON CONFLICT (name) DO NOTHING",
                    (tag_name,),
                )
                cur.execute("SELECT id FROM tags WHERE name = %s", (tag_name,))
                tag_id = cur.fetchone()[0]
                cur.execute(
                    "INSERT INTO file_tags (file_id, tag_id) VALUES (%s, %s) ON CONFLICT DO NOTHING",
                    (file_id, tag_id),
                )

        conn.commit()
