"""Benchmark ApoFocus's real local analysis endpoint with photographer-owned media.

The script sends filesystem paths, never media bytes. Its output directory must
be inside THUMBNAIL_ROOTS because the embedding service writes temporary
thumbnails and keyframes there.
"""

from __future__ import annotations

import argparse
import json
import math
import statistics
import tempfile
import time
import urllib.error
import urllib.request
from collections import defaultdict
from pathlib import Path


PHOTO_EXTENSIONS = {
    ".jpg", ".jpeg", ".png", ".webp", ".tif", ".tiff", ".bmp",
    ".dng", ".arw", ".cr2", ".cr3", ".nef", ".raf", ".3fr", ".orf", ".rw2", ".pef",
}
VIDEO_EXTENSIONS = {".mp4", ".mov", ".m4v", ".mkv", ".avi", ".webm", ".mts", ".m2ts"}
AUDIO_EXTENSIONS = {".wav", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".opus", ".aiff", ".aif"}


def media_type(path: Path) -> str:
    extension = path.suffix.lower()
    if extension in PHOTO_EXTENSIONS:
        return "photo"
    if extension in VIDEO_EXTENSIONS:
        return "video"
    if extension in AUDIO_EXTENSIONS:
        return "audio"
    return ""


def discover(source: Path, recursive: bool, selected: set[str], limit_per_type: int) -> dict[str, list[Path]]:
    result: dict[str, list[Path]] = {kind: [] for kind in sorted(selected)}
    candidates = source.rglob("*") if recursive else source.glob("*")
    for path in sorted(candidates):
        if not path.is_file():
            continue
        kind = media_type(path)
        if kind not in selected:
            continue
        if limit_per_type > 0 and len(result[kind]) >= limit_per_type:
            continue
        result[kind].append(path.resolve())
    return result


def request_analysis(service_url: str, kind: str, source: Path, work_dir: Path, index: int) -> dict:
    item_dir = work_dir / f"{kind}-{index:06d}"
    item_dir.mkdir(parents=True, exist_ok=True)
    payload = {"path": str(source), "thumbnailPath": str(item_dir / "thumbnail.avif")}
    endpoint = "/v1/analyze"
    if kind != "photo":
        endpoint = "/v1/analyze-media"
        payload.update({"segmentDir": str(item_dir / "segments"), "autoTags": True})
    request = urllib.request.Request(
        service_url.rstrip("/") + endpoint,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    started = time.perf_counter()
    try:
        with urllib.request.urlopen(request, timeout=12 * 60 * 60) as response:
            body = json.load(response)
    except urllib.error.HTTPError as error:
        detail = error.read().decode("utf-8", errors="replace")[-1200:]
        raise RuntimeError(f"HTTP {error.code}: {detail}") from error
    wall_ms = round((time.perf_counter() - started) * 1000, 3)
    return {"wallMs": wall_ms, "timingsMs": body.get("timingsMs", {})}


def percentile(values: list[float], fraction: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    index = max(0, min(len(ordered) - 1, math.ceil(fraction * len(ordered)) - 1))
    return ordered[index]


def summarize(records: list[dict]) -> dict:
    successes = [record for record in records if "error" not in record]
    walls = [float(record["wallMs"]) for record in successes]
    stages: dict[str, list[float]] = defaultdict(list)
    for record in successes:
        for name, value in record.get("timingsMs", {}).items():
            stages[name].append(float(value))
    return {
        "count": len(records),
        "succeeded": len(successes),
        "failed": len(records) - len(successes),
        "wallMs": {
            "total": round(sum(walls), 3),
            "mean": round(statistics.fmean(walls), 3) if walls else 0.0,
            "p50": round(percentile(walls, 0.50), 3),
            "p95": round(percentile(walls, 0.95), 3),
        },
        "stagesMs": {
            name: {
                "total": round(sum(values), 3),
                "mean": round(statistics.fmean(values), 3),
                "p95": round(percentile(values, 0.95), 3),
            }
            for name, values in sorted(stages.items())
        },
    }


def backend_decision(photo_records: list[dict], threshold_percent: float) -> dict:
    successes = [record for record in photo_records if "error" not in record]
    thumbnail_ms = sum(float(record.get("timingsMs", {}).get("thumbnailMs", 0)) for record in successes)
    total_ms = sum(float(record.get("timingsMs", {}).get("totalMs", 0)) for record in successes)
    share = thumbnail_ms / total_ms * 100 if total_ms else 0.0
    enough_data = len(successes) >= 20
    evaluate = enough_data and share >= threshold_percent
    return {
        "sampleCount": len(successes),
        "thresholdPercent": threshold_percent,
        "thumbnailSharePercent": round(share, 3),
        "decision": "evaluate_libvips" if evaluate else "keep_pillow",
        "enoughData": enough_data,
        "reason": (
            "Stored thumbnail resize/encode exceeds the threshold; benchmark libvips on the same corpus."
            if evaluate else
            "Keep Pillow: the replaceable thumbnail stage is below the threshold or fewer than 20 photos were measured."
        ),
        "scope": "The ratio excludes shared decode, OpenCLIP inference, tagging, and dominant-color work.",
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Benchmark ApoFocus photo/video/audio analysis on local media")
    parser.add_argument("--source", required=True, type=Path, help="folder of allowlisted media")
    parser.add_argument("--output-dir", required=True, type=Path, help="temporary output folder inside THUMBNAIL_ROOTS")
    parser.add_argument("--service-url", default="http://127.0.0.1:8090")
    parser.add_argument("--media", action="append", choices=("photo", "video", "audio"), help="repeatable; defaults to all")
    parser.add_argument("--limit-per-type", type=int, default=100, help="0 means all discovered files")
    parser.add_argument("--recursive", action=argparse.BooleanOptionalAction, default=True)
    parser.add_argument("--warmup", type=int, default=1, help="excluded requests per media type before measurement")
    parser.add_argument("--backend-threshold-percent", type=float, default=20.0)
    parser.add_argument("--json", type=Path, help="optional report path")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    source = args.source.expanduser().resolve(strict=True)
    if not source.is_dir():
        raise SystemExit("--source must be a directory")
    output_dir = args.output_dir.expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    selected = set(args.media or ("photo", "video", "audio"))
    discovered = discover(source, args.recursive, selected, max(0, args.limit_per_type))
    records: dict[str, list[dict]] = {kind: [] for kind in sorted(selected)}

    with tempfile.TemporaryDirectory(prefix="apofocus-benchmark-", dir=output_dir) as temporary:
        work_dir = Path(temporary)
        for kind in sorted(selected):
            paths = discovered.get(kind, [])
            for warmup_index, path in enumerate(paths[:max(0, args.warmup)]):
                print(f"warmup {kind}: {path.name}", flush=True)
                request_analysis(args.service_url, kind, path, work_dir, -(warmup_index + 1))
            for index, path in enumerate(paths):
                print(f"benchmark {kind} {index + 1}/{len(paths)}: {path.name}", flush=True)
                try:
                    result = request_analysis(args.service_url, kind, path, work_dir, index)
                    result["path"] = str(path)
                except Exception as error:  # Preserve the rest of a large corpus run.
                    result = {"path": str(path), "error": str(error)}
                records[kind].append(result)

    report = {
        "source": str(source),
        "serviceUrl": args.service_url,
        "results": {kind: summarize(values) for kind, values in records.items()},
        "thumbnailBackendDecision": backend_decision(records.get("photo", []), args.backend_threshold_percent),
        "failures": {
            kind: [{"path": record["path"], "error": record["error"]} for record in values if "error" in record]
            for kind, values in records.items()
        },
    }
    rendered = json.dumps(report, ensure_ascii=False, indent=2)
    print(rendered)
    if args.json:
        destination = args.json.expanduser().resolve()
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(rendered + "\n", encoding="utf-8")
    return 0 if any(summary["succeeded"] for summary in report["results"].values()) else 1


if __name__ == "__main__":
    raise SystemExit(main())
