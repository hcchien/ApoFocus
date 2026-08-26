"""Batch missing ApoFocus embeddings into PostgreSQL.

Usage: DATABASE_URL=... PHOTO_ROOTS=/Volumes/archive python worker.py
"""

from __future__ import annotations

import os
from itertools import islice

import psycopg

from app import embed, safe_path


DATABASE_URL = os.environ["DATABASE_URL"]
BATCH_SIZE = int(os.getenv("EMBEDDING_BATCH_SIZE", "16"))


def vector_literal(vector: list[float]) -> str:
    return "[" + ",".join(f"{value:.8f}" for value in vector) + "]"


def batches(iterable, size: int):
    iterator = iter(iterable)
    while batch := list(islice(iterator, size)):
        yield batch


def main() -> None:
    with psycopg.connect(DATABASE_URL) as connection:
        rows = connection.execute(
            "SELECT id, path FROM photos WHERE embedding IS NULL ORDER BY taken_at"
        ).fetchall()
        for batch in batches(rows, BATCH_SIZE):
            paths = [safe_path(path) for _, path in batch]
            vectors = embed(paths)
            with connection.transaction():
                connection.executemany(
                    "UPDATE photos SET embedding=%s::vector, updated_at=now() WHERE id=%s",
                    [(vector_literal(vector), photo_id) for (photo_id, _), vector in zip(batch, vectors)],
                )
            print(f"indexed {len(batch)} photos")
    print(f"done: {len(rows)} embeddings indexed")


if __name__ == "__main__":
    main()
