import os

from psycopg2 import pool

_pool = None


def _get_pool():
    global _pool
    if _pool is None or _pool.closed:
        _pool = pool.ThreadedConnectionPool(
            minconn=1,
            maxconn=4,
            host=os.environ.get("PGHOST", "/tmp"),
            dbname="engram",
        )
    return _pool


def _get_conn():
    return _get_pool().getconn()


def _put_conn(conn):
    try:
        _get_pool().putconn(conn)
    except Exception:
        pass


def upsert_file(
    file_path: str,
    filename: str,
    size: int,
    hash_value: str,
    mtime: str,
    device_name: str,
    storage_type: str,
    owner: str | None = None,
) -> str:
    """Create or reset a file record identified by storage type and path."""
    conn = _get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute(
                """INSERT INTO devices (name) VALUES (%s)
                   ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
                   RETURNING id""",
                (device_name,),
            )
            device_id = cur.fetchone()[0]

            cur.execute(
                """INSERT INTO files (filename, size, hash, file_path, device_id, status, storage_type, mtime, owner)
                   VALUES (%s, %s, %s, %s, %s, 'pending', %s, %s, %s)
                   ON CONFLICT (storage_type, file_path) DO UPDATE SET
                       filename = EXCLUDED.filename,
                       size = EXCLUDED.size,
                       hash = EXCLUDED.hash,
                       device_id = EXCLUDED.device_id,
                       status = 'pending',
                       mtime = EXCLUDED.mtime,
                       owner = EXCLUDED.owner,
                       mime_type = NULL,
                       page_count = NULL,
                       extracted_text = NULL,
                       updated_at = now()
                   RETURNING id""",
                (
                    filename,
                    size,
                    hash_value,
                    file_path,
                    device_id,
                    storage_type,
                    mtime,
                    owner,
                ),
            )
            file_id = str(cur.fetchone()[0])
            cur.execute("DELETE FROM file_tags WHERE file_id = %s", (file_id,))

        conn.commit()
        return file_id
    except Exception:
        conn.rollback()
        raise
    finally:
        _put_conn(conn)


def update_file_status(file_id: str, status: str):
    conn = _get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE files SET status = %s, updated_at = now() WHERE id = %s",
                (status, file_id),
            )
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        _put_conn(conn)


def delete_file(storage_type: str, file_path: str):
    """Delete a file record by its storage identity."""
    conn = _get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute(
                "DELETE FROM files WHERE storage_type = %s AND file_path = %s",
                (storage_type, file_path),
            )
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        _put_conn(conn)


def rename_file(
    storage_type: str,
    old_file_path: str,
    new_file_path: str,
    new_filename: str,
):
    """Update the path and filename for a stored file."""
    conn = _get_conn()
    try:
        with conn.cursor() as cur:
            cur.execute(
                """UPDATE files
                   SET file_path = %s, filename = %s, updated_at = now()
                   WHERE storage_type = %s AND file_path = %s""",
                (new_file_path, new_filename, storage_type, old_file_path),
            )
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        _put_conn(conn)


def update_file_metadata(
    file_id: str,
    mime_type: str,
    page_count: int | None,
    extracted_text: str | None,
    tags: list[str],
):
    conn = _get_conn()
    try:
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
    except Exception:
        conn.rollback()
        raise
    finally:
        _put_conn(conn)
