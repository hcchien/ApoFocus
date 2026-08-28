"""Local photo, video, and audio analysis service for ApoFocus.

This service accepts filesystem paths only from configured roots. Run it next
to the Go API; do not expose it directly to the public web.
"""

from __future__ import annotations

import os
import json
import importlib.util
import io
import mimetypes
import shutil
import subprocess
import tempfile
import time
from datetime import datetime, timezone
from functools import lru_cache
from pathlib import Path

import open_clip
import rawpy
import torch
from fastapi import FastAPI, HTTPException
from PIL import Image, ImageOps, features
from pydantic import BaseModel, Field

WHISPER_INSTALLED = importlib.util.find_spec("whisper") is not None
CLAP_INSTALLED = importlib.util.find_spec("laion_clap") is not None


MODEL_NAME = os.getenv("EMBEDDING_MODEL", "ViT-B-32")
MODEL_WEIGHTS = os.getenv("EMBEDDING_WEIGHTS", "laion2b_s34b_b79k")
PHOTO_ROOTS = tuple(
    Path(value).expanduser().resolve()
    for value in os.getenv("PHOTO_ROOTS", os.getenv("APOFOCUS_IMPORT_ROOTS", "./photos")).split(os.pathsep)
    if value
)
THUMBNAIL_ROOTS = tuple(
    Path(value).expanduser().resolve()
    for value in os.getenv("THUMBNAIL_ROOTS", os.getenv("PHOTO_LIBRARY_ROOT", "./photos")).split(os.pathsep)
    if value
)
DEVICE = "mps" if torch.backends.mps.is_available() else "cuda" if torch.cuda.is_available() else "cpu"
WHISPER_DEVICE = "cuda" if torch.cuda.is_available() else "cpu"
AUTO_TAG_LIMIT = int(os.getenv("AUTO_TAG_LIMIT", "4"))
AUTO_TAG_MIN_SCORE = float(os.getenv("AUTO_TAG_MIN_SCORE", "0.18"))
WHISPER_MODEL = os.getenv("WHISPER_MODEL", "base")
WHISPER_LANGUAGE = os.getenv("WHISPER_LANGUAGE", "")
WHISPER_DOWNLOAD_ROOT = os.getenv("WHISPER_DOWNLOAD_ROOT", "")
VIDEO_SAMPLE_SECONDS = max(1, int(os.getenv("VIDEO_SAMPLE_SECONDS", "10")))
MAX_VIDEO_SEGMENTS = max(1, int(os.getenv("MAX_VIDEO_SEGMENTS", "300")))
AUDIO_SEGMENT_SECONDS = max(5, int(os.getenv("AUDIO_SEGMENT_SECONDS", "30")))
MAX_AUDIO_SEGMENTS = max(1, int(os.getenv("MAX_AUDIO_SEGMENTS", "600")))
CLAP_EXPECTED_DIMENSIONS = int(os.getenv("CLAP_EXPECTED_DIMENSIONS", "512"))
VIDEO_KEYFRAME_MAX_EDGE = max(320, int(os.getenv("VIDEO_KEYFRAME_MAX_EDGE", "960")))
PHOTO_THUMBNAIL_MAX_EDGE = max(320, int(os.getenv("PHOTO_THUMBNAIL_MAX_EDGE", "1600")))
DERIVATIVE_IMAGE_EXTENSION = ".avif"
DERIVATIVE_IMAGE_QUALITY = min(100, max(1, int(os.getenv("DERIVATIVE_IMAGE_QUALITY", "42"))))
DERIVATIVE_IMAGE_SPEED = min(10, max(0, int(os.getenv("DERIVATIVE_IMAGE_SPEED", "6"))))
RAW_THUMBNAIL_MAX_EDGE = max(320, int(os.getenv("RAW_THUMBNAIL_MAX_EDGE", "960")))
RAW_THUMBNAIL_QUALITY = min(100, max(1, int(os.getenv("RAW_THUMBNAIL_QUALITY", "36"))))
RAW_EXTENSIONS = {".dng", ".arw", ".cr2", ".cr3", ".nef", ".raf", ".3fr", ".orf", ".rw2", ".pef"}

TAG_CANDIDATES = {
    "portrait": "肖像",
    "people": "人物",
    "street photography": "街頭",
    "city": "城市",
    "architecture": "建築",
    "interior": "室內",
    "mountain": "山林",
    "forest": "森林",
    "ocean or coast": "海岸",
    "river or lake": "水域",
    "sunset or sunrise": "晨昏",
    "night photography": "夜間",
    "food": "食物",
    "animal": "動物",
    "sports": "運動",
    "concert or performance": "表演",
    "wedding": "婚禮",
    "transportation": "交通",
    "documentary photography": "紀實",
    "minimalism": "極簡",
}

AUDIO_TAG_CANDIDATES = {
    "speech or conversation": "對話",
    "interview": "訪談",
    "applause or crowd": "群眾",
    "live music or concert": "現場音樂",
    "instrumental music": "器樂",
    "traffic and street noise": "交通聲",
    "ocean waves or water": "水聲",
    "rain or thunder": "雨聲",
    "wind in nature": "風聲",
    "birds or animals": "動物聲",
    "indoor room ambience": "室內環境",
    "silence or very quiet ambience": "安靜",
}

app = FastAPI(title="ApoFocus Embeddings", version="1.0.0")


class EmbedRequest(BaseModel):
    paths: list[str] = Field(min_length=1, max_length=64)


class EmbeddingItem(BaseModel):
    path: str
    vector: list[float]


class EmbedResponse(BaseModel):
    model: str
    dimensions: int
    items: list[EmbeddingItem]


class AnalyzeRequest(BaseModel):
    path: str
    thumbnail_path: str = Field(default="", alias="thumbnailPath")


class AnalyzeResponse(BaseModel):
    vector: list[float]
    tags: list[str]
    dominant_color: str = Field(alias="dominantColor")
    timings_ms: dict[str, float] = Field(default_factory=dict, alias="timingsMs")

class AnalyzeBatchRequest(BaseModel):
    items: list[AnalyzeRequest] = Field(min_length=1, max_length=16)

class AnalyzeBatchItemResponse(AnalyzeResponse):
    path: str

class AnalyzeBatchResponse(BaseModel):
    items: list[AnalyzeBatchItemResponse]


class AnalyzeMediaRequest(BaseModel):
    path: str
    thumbnail_path: str = Field(default="", alias="thumbnailPath")
    segment_dir: str = Field(default="", alias="segmentDir")
    auto_tags: bool = Field(default=True, alias="autoTags")


class MediaSegmentResponse(BaseModel):
    segment_type: str = Field(alias="segmentType")
    index: int
    start_ms: int = Field(alias="startMs")
    end_ms: int = Field(alias="endMs")
    keyframe_path: str = Field(default="", alias="keyframePath")
    transcript: str = ""
    tags: list[str] = Field(default_factory=list)
    visual_vector: list[float] = Field(default_factory=list, alias="visualVector")
    audio_vector: list[float] = Field(default_factory=list, alias="audioVector")


class AnalyzeMediaResponse(BaseModel):
    media_type: str = Field(alias="mediaType")
    duration_ms: int = Field(alias="durationMs")
    mime_type: str = Field(alias="mimeType")
    codec: str
    dimensions: str = ""
    sample_rate: int = Field(default=0, alias="sampleRate")
    channels: int = 0
    recorded_at: str = Field(alias="recordedAt")
    transcript: str = ""
    tags: list[str] = Field(default_factory=list)
    metadata: dict = Field(default_factory=dict)
    segments: list[MediaSegmentResponse] = Field(default_factory=list)
    timings_ms: dict[str, float] = Field(default_factory=dict, alias="timingsMs")


@lru_cache(maxsize=1)
def load_model():
    model, _, preprocess = open_clip.create_model_and_transforms(
        MODEL_NAME, pretrained=MODEL_WEIGHTS, device=DEVICE
    )
    model.eval()
    return model, preprocess


@lru_cache(maxsize=1)
def tag_features():
    model, _ = load_model()
    tokenizer = open_clip.get_tokenizer(MODEL_NAME)
    prompts = [f"a professional photograph of {label}" for label in TAG_CANDIDATES]
    tokens = tokenizer(prompts).to(DEVICE)
    with torch.inference_mode():
        features = model.encode_text(tokens)
        return features / features.norm(dim=-1, keepdim=True)


def safe_path(value: str) -> Path:
    candidate = Path(value).expanduser().resolve(strict=True)
    if not any(candidate.is_relative_to(root) for root in PHOTO_ROOTS):
        raise HTTPException(status_code=403, detail=f"path is outside PHOTO_ROOTS: {value}")
    if not candidate.is_file():
        raise HTTPException(status_code=400, detail=f"not a regular media file: {value}")
    return candidate


def safe_thumbnail_path(value: str) -> Path:
    candidate = Path(value).expanduser().resolve()
    if not any(candidate.is_relative_to(root) for root in THUMBNAIL_ROOTS):
        raise HTTPException(status_code=403, detail="thumbnailPath is outside THUMBNAIL_ROOTS")
    if candidate.suffix.lower() != DERIVATIVE_IMAGE_EXTENSION:
        raise HTTPException(status_code=400, detail=f"thumbnailPath must end in {DERIVATIVE_IMAGE_EXTENSION}")
    candidate.parent.mkdir(parents=True, exist_ok=True, mode=0o750)
    resolved_parent = candidate.parent.resolve(strict=True)
    if not any(resolved_parent.is_relative_to(root) for root in THUMBNAIL_ROOTS):
        raise HTTPException(status_code=403, detail="thumbnailPath parent escapes THUMBNAIL_ROOTS")
    return candidate


def load_image(path: Path) -> Image.Image:
    try:
        with Image.open(path) as image:
            return ImageOps.exif_transpose(image).convert("RGB")
    except (OSError, ValueError) as pillow_error:
        try:
            with rawpy.imread(str(path)) as raw:
                try:
                    preview = raw.extract_thumb()
                    if preview.format == rawpy.ThumbFormat.JPEG:
                        with Image.open(io.BytesIO(preview.data)) as image:
                            return ImageOps.exif_transpose(image).convert("RGB")
                    if preview.format == rawpy.ThumbFormat.BITMAP:
                        return Image.fromarray(preview.data).convert("RGB")
                except (rawpy.LibRawError, OSError, ValueError):
                    pass
                pixels = raw.postprocess(use_camera_wb=True, half_size=True, no_auto_bright=False)
                return Image.fromarray(pixels).convert("RGB")
        except (rawpy.LibRawError, OSError, ValueError) as raw_error:
            raise HTTPException(status_code=422, detail=f"cannot decode {path}: {pillow_error}; RAW fallback: {raw_error}") from raw_error


def make_thumbnail(image: Image.Image, max_edge: int) -> Image.Image:
    thumbnail = image.copy()
    thumbnail.thumbnail((max_edge, max_edge), Image.Resampling.LANCZOS)
    return thumbnail


def save_derivative(image: Image.Image, destination: Path, quality: int | None = None) -> None:
    if not features.check("avif"):
        raise HTTPException(status_code=503, detail="Pillow AVIF support is required for derivative images")
    handle = tempfile.NamedTemporaryFile(dir=destination.parent, suffix=DERIVATIVE_IMAGE_EXTENSION, delete=False)
    temporary = Path(handle.name)
    handle.close()
    try:
        image.save(
            temporary,
            format="AVIF",
            quality=quality if quality is not None else DERIVATIVE_IMAGE_QUALITY,
            speed=DERIVATIVE_IMAGE_SPEED,
        )
        os.chmod(temporary, 0o640)
        os.replace(temporary, destination)
    finally:
        temporary.unlink(missing_ok=True)


def dominant_color(image: Image.Image) -> str:
    red, green, blue = image.resize((1, 1), Image.Resampling.BOX).getpixel((0, 0))
    return f"#{red:02x}{green:02x}{blue:02x}"


def make_video_thumbnail(images: list[Image.Image]) -> Image.Image:
    selected = images[:2]
    if len(selected) == 1:
        return make_thumbnail(selected[0], VIDEO_KEYFRAME_MAX_EDGE)
    height = max(180, round(VIDEO_KEYFRAME_MAX_EDGE * 9 / 16))
    cell_width = VIDEO_KEYFRAME_MAX_EDGE // 2
    canvas = Image.new("RGB", (VIDEO_KEYFRAME_MAX_EDGE, height), "black")
    for index, image in enumerate(selected):
        fitted = image.copy()
        fitted.thumbnail((cell_width, height), Image.Resampling.LANCZOS)
        x = index * cell_width + (cell_width - fitted.width) // 2
        y = (height - fitted.height) // 2
        canvas.paste(fitted, (x, y))
    return canvas


def embed_images(images: list[Image.Image]) -> list[list[float]]:
    model, preprocess = load_model()
    tensors = [preprocess(image) for image in images]

    batch = torch.stack(tensors).to(DEVICE)
    with torch.inference_mode():
        vectors = model.encode_image(batch)
        vectors = vectors / vectors.norm(dim=-1, keepdim=True)
    return vectors.cpu().float().tolist()


def embed(paths: list[Path]) -> list[list[float]]:
    return embed_images([load_image(path) for path in paths])


def classify(vector: list[float]) -> list[str]:
    image_features = torch.tensor(vector, device=DEVICE).unsqueeze(0)
    similarities = image_features @ tag_features().T
    scores, indexes = similarities[0].topk(min(AUTO_TAG_LIMIT, len(TAG_CANDIDATES)))
    ranked = list(zip(scores.cpu().tolist(), indexes.cpu().tolist()))
    labels = list(TAG_CANDIDATES.values())
    selected = [labels[index] for score, index in ranked if score >= AUTO_TAG_MIN_SCORE]
    return selected or [labels[ranked[0][1]]]


def require_binary(name: str) -> str:
    binary = shutil.which(name)
    if not binary:
        raise HTTPException(status_code=503, detail=f"{name} is required for video/audio analysis")
    return binary


def run_command(arguments: list[str], operation: str) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(arguments, check=True, capture_output=True, text=True)
    except subprocess.CalledProcessError as error:
        detail = (error.stderr or error.stdout or str(error)).strip()[-1200:]
        raise HTTPException(status_code=422, detail=f"{operation} failed: {detail}") from error


def elapsed_ms(started: float) -> float:
    return round((time.perf_counter() - started) * 1000, 3)


def probe_media(path: Path) -> dict:
    result = run_command(
        [require_binary("ffprobe"), "-v", "error", "-show_format", "-show_streams", "-of", "json", str(path)],
        "ffprobe",
    )
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as error:
        raise HTTPException(status_code=422, detail="ffprobe returned invalid JSON") from error


def media_description(path: Path, probe: dict) -> dict:
    streams = probe.get("streams", [])
    video_stream = next(
        (stream for stream in streams if stream.get("codec_type") == "video" and not stream.get("disposition", {}).get("attached_pic")),
        None,
    )
    audio_stream = next((stream for stream in streams if stream.get("codec_type") == "audio"), None)
    if video_stream:
        media_type = "video"
        primary = video_stream
    elif audio_stream:
        media_type = "audio"
        primary = audio_stream
    else:
        raise HTTPException(status_code=422, detail="file contains neither a video nor an audio stream")
    format_info = probe.get("format", {})
    duration_value = format_info.get("duration") or primary.get("duration") or 0
    try:
        duration_ms = max(0, round(float(duration_value) * 1000))
    except (TypeError, ValueError):
        duration_ms = 0
    tags = {**format_info.get("tags", {}), **primary.get("tags", {})}
    recorded_at = tags.get("creation_time") or tags.get("date")
    if not recorded_at:
        recorded_at = datetime.fromtimestamp(path.stat().st_mtime, timezone.utc).isoformat()
    mime_type = mimetypes.guess_type(path.name)[0] or ("video/unknown" if media_type == "video" else "audio/unknown")
    dimensions = ""
    if video_stream:
        dimensions = f"{video_stream.get('width', 0)} × {video_stream.get('height', 0)}"
    return {
        "media_type": media_type,
        "duration_ms": duration_ms,
        "mime_type": mime_type,
        "codec": str(primary.get("codec_name", "")),
        "dimensions": dimensions,
        "sample_rate": int(audio_stream.get("sample_rate") or 0) if audio_stream else 0,
        "channels": int(audio_stream.get("channels") or 0) if audio_stream else 0,
        "recorded_at": recorded_at,
        "has_audio": audio_stream is not None,
    }


def ffmpeg_extract_frame(source: Path, seconds: float, destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True, mode=0o750)
    run_command(
        [
            require_binary("ffmpeg"), "-v", "error", "-y", "-ss", f"{seconds:.3f}", "-i", str(source),
            "-frames:v", "1", "-vf", video_keyframe_filter(), "-q:v", "2", str(destination),
        ],
        "video frame extraction",
    )
    if not destination.is_file():
        run_command(
            [
                require_binary("ffmpeg"), "-v", "error", "-y", "-sseof", "-1", "-i", str(source),
                "-frames:v", "1", "-vf", video_keyframe_filter(), "-q:v", "2", str(destination),
            ],
            "video tail frame extraction",
        )
    if not destination.is_file():
        raise HTTPException(status_code=422, detail="video frame extraction produced no image")
    os.chmod(destination, 0o640)


def video_keyframe_filter() -> str:
    edge = VIDEO_KEYFRAME_MAX_EDGE
    return (
        f"scale='min({edge},iw)':'min({edge},ih)':"
        "force_original_aspect_ratio=decrease:force_divisible_by=2"
    )


def ffmpeg_extract_frames(source: Path, timestamps_ms: list[int], destination: Path) -> list[Path]:
    destination.mkdir(parents=True, exist_ok=True, mode=0o750)
    if len(timestamps_ms) == 1:
        frame = destination / "frame-000000.jpg"
        ffmpeg_extract_frame(source, timestamps_ms[0] / 1000, frame)
        return [frame]
    output_pattern = destination / "frame-%06d.jpg"
    run_command(
        [
            require_binary("ffmpeg"), "-v", "error", "-y",
            "-ss", f"{timestamps_ms[0] / 1000:.3f}", "-i", str(source),
            "-vf", f"fps=1/{VIDEO_SAMPLE_SECONDS}:round=up:eof_action=pass,{video_keyframe_filter()}",
            "-frames:v", str(len(timestamps_ms)), "-q:v", "2", "-start_number", "0", str(output_pattern),
        ],
        "video frame extraction",
    )
    frames = [destination / f"frame-{index:06d}.jpg" for index in range(len(timestamps_ms))]
    for index, frame in enumerate(frames):
        if not frame.is_file():
            ffmpeg_extract_frame(source, timestamps_ms[index] / 1000, frame)
    for frame in frames:
        os.chmod(frame, 0o640)
    return frames


def ffmpeg_extract_audio(source: Path, destination: Path, start_seconds: float | None = None, duration_seconds: float | None = None) -> None:
    arguments = [require_binary("ffmpeg"), "-v", "error", "-y"]
    if start_seconds is not None:
        arguments += ["-ss", f"{start_seconds:.3f}"]
    arguments += ["-i", str(source)]
    if duration_seconds is not None:
        arguments += ["-t", f"{duration_seconds:.3f}"]
    arguments += ["-vn", "-ac", "1", "-ar", "48000", "-c:a", "pcm_s16le", str(destination)]
    run_command(arguments, "audio extraction")


@lru_cache(maxsize=1)
def load_whisper_model():
    if not WHISPER_INSTALLED:
        raise HTTPException(status_code=503, detail="openai-whisper is not installed")
    import whisper as whisper_module
    options = {"device": WHISPER_DEVICE}
    if WHISPER_DOWNLOAD_ROOT:
        Path(WHISPER_DOWNLOAD_ROOT).mkdir(parents=True, exist_ok=True)
        options["download_root"] = WHISPER_DOWNLOAD_ROOT
    return whisper_module.load_model(WHISPER_MODEL, **options)


def transcribe_audio(path: Path) -> tuple[str, list[dict]]:
    options = {"verbose": False, "fp16": WHISPER_DEVICE == "cuda", "task": "transcribe"}
    if WHISPER_LANGUAGE:
        options["language"] = WHISPER_LANGUAGE
    result = load_whisper_model().transcribe(str(path), **options)
    segments = [
        {
            "start_ms": max(0, round(float(segment.get("start", 0)) * 1000)),
            "end_ms": max(0, round(float(segment.get("end", 0)) * 1000)),
            "text": str(segment.get("text", "")).strip(),
        }
        for segment in result.get("segments", [])
        if str(segment.get("text", "")).strip()
    ]
    return str(result.get("text", "")).strip(), segments


@lru_cache(maxsize=1)
def load_clap_model():
    if not CLAP_INSTALLED:
        raise HTTPException(status_code=503, detail="laion-clap is not installed")
    import laion_clap as clap_module
    model = clap_module.CLAP_Module(enable_fusion=False)
    model.load_ckpt()
    return model


def normalize_vector(values) -> list[float]:
    vector = torch.as_tensor(values, dtype=torch.float32).flatten()
    norm = vector.norm()
    if norm <= 0:
        raise HTTPException(status_code=422, detail="model returned a zero vector")
    result = (vector / norm).cpu().tolist()
    return result


@lru_cache(maxsize=1)
def clap_tag_features():
    prompts = [f"an audio recording of {label}" for label in AUDIO_TAG_CANDIDATES]
    features = load_clap_model().get_text_embedding(prompts, use_tensor=True)
    features = torch.as_tensor(features, dtype=torch.float32)
    return features / features.norm(dim=-1, keepdim=True)


def embed_audio_files(paths: list[Path]) -> list[list[float]]:
    if not paths:
        return []
    vectors = []
    for offset in range(0, len(paths), 8):
        batch = paths[offset:offset + 8]
        features = load_clap_model().get_audio_embedding_from_filelist(x=[str(path) for path in batch], use_tensor=True)
        vectors.extend(normalize_vector(feature) for feature in features)
    if any(len(vector) != CLAP_EXPECTED_DIMENSIONS for vector in vectors):
        dimensions = sorted({len(vector) for vector in vectors})
        raise HTTPException(status_code=422, detail=f"CLAP returned dimensions {dimensions}; expected {CLAP_EXPECTED_DIMENSIONS}")
    return vectors


def classify_audio(vector: list[float]) -> list[str]:
    features = torch.tensor(vector, dtype=torch.float32).unsqueeze(0)
    similarities = features @ clap_tag_features().cpu().T
    _, indexes = similarities[0].topk(min(AUTO_TAG_LIMIT, len(AUDIO_TAG_CANDIDATES)))
    labels = list(AUDIO_TAG_CANDIDATES.values())
    return [labels[index] for index in indexes.tolist()]


def overlapping_transcript(start_ms: int, end_ms: int, transcript_segments: list[dict]) -> str:
    return " ".join(
        segment["text"] for segment in transcript_segments
        if segment["start_ms"] < end_ms and segment["end_ms"] > start_ms
    ).strip()


def visual_segments(
    source: Path,
    duration_ms: int,
    temporary_dir: Path,
    thumbnail_path: Path | None,
    auto_tags: bool,
) -> tuple[list[MediaSegmentResponse], dict[str, float]]:
    if duration_ms <= 0:
        timestamps = [0]
    else:
        count = min(MAX_VIDEO_SEGMENTS, max(1, (duration_ms + VIDEO_SAMPLE_SECONDS * 1000 - 1) // (VIDEO_SAMPLE_SECONDS * 1000)))
        interval_ms = VIDEO_SAMPLE_SECONDS * 1000
        latest_safe_ms = max(0, duration_ms - 1000)
        timestamps = [min(latest_safe_ms, index * interval_ms + interval_ms // 2) for index in range(count)]
    stage_started = time.perf_counter()
    frame_paths = ffmpeg_extract_frames(source, timestamps, temporary_dir)
    extraction_ms = elapsed_ms(stage_started)

    vectors = []
    decoding_ms = 0.0
    embedding_ms = 0.0
    thumbnail_compose_ms = 0.0
    thumbnail_encode_ms = 0.0
    thumbnail_indexes = {0, len(frame_paths) // 2} if len(frame_paths) > 1 else {0}
    thumbnail_candidates: dict[int, Image.Image] = {}
    for offset in range(0, len(frame_paths), 16):
        batch_paths = frame_paths[offset:offset + 16]
        stage_started = time.perf_counter()
        images = [load_image(path) for path in batch_paths]
        decoding_ms += elapsed_ms(stage_started)
        for batch_index, image in enumerate(images):
            global_index = offset + batch_index
            if global_index in thumbnail_indexes:
                thumbnail_candidates[global_index] = image.copy()
        stage_started = time.perf_counter()
        vectors.extend(embed_images(images))
        embedding_ms += elapsed_ms(stage_started)

    if thumbnail_path is not None and thumbnail_candidates:
        stage_started = time.perf_counter()
        thumbnail = make_video_thumbnail([thumbnail_candidates[index] for index in sorted(thumbnail_candidates)])
        thumbnail_compose_ms = elapsed_ms(stage_started)
        stage_started = time.perf_counter()
        save_derivative(thumbnail, thumbnail_path)
        thumbnail_encode_ms = elapsed_ms(stage_started)

    stage_started = time.perf_counter()
    segments = []
    for index, vector in enumerate(vectors):
        start_ms = index * VIDEO_SAMPLE_SECONDS * 1000
        end_ms = min(duration_ms, start_ms + VIDEO_SAMPLE_SECONDS * 1000) if duration_ms else start_ms
        segments.append(MediaSegmentResponse(
            segmentType="visual", index=index, startMs=start_ms, endMs=max(start_ms, end_ms),
            keyframePath="", tags=classify(vector) if auto_tags else [], visualVector=vector,
        ))
    return segments, {
        "keyframeExtractionMs": extraction_ms,
        "visualDecodingMs": round(decoding_ms, 3),
        "visualEmbeddingMs": round(embedding_ms, 3),
        "thumbnailComposeMs": thumbnail_compose_ms,
        "thumbnailEncodeMs": thumbnail_encode_ms,
        "visualTaggingMs": elapsed_ms(stage_started),
    }


def audio_segments(source_audio: Path, duration_ms: int, transcript_segments: list[dict], temporary_dir: Path, auto_tags: bool) -> tuple[list[MediaSegmentResponse], dict[str, float]]:
    if duration_ms <= 0:
        intervals = [(0, AUDIO_SEGMENT_SECONDS * 1000)]
    else:
        count = min(MAX_AUDIO_SEGMENTS, max(1, (duration_ms + AUDIO_SEGMENT_SECONDS * 1000 - 1) // (AUDIO_SEGMENT_SECONDS * 1000)))
        intervals = [
            (index * AUDIO_SEGMENT_SECONDS * 1000, min(duration_ms, (index + 1) * AUDIO_SEGMENT_SECONDS * 1000))
            for index in range(count)
        ]
    stage_started = time.perf_counter()
    chunks = []
    for index, (start_ms, end_ms) in enumerate(intervals):
        chunk = temporary_dir / f"audio-{index:06d}.wav"
        ffmpeg_extract_audio(source_audio, chunk, start_ms / 1000, max(0.001, (end_ms - start_ms) / 1000))
        chunks.append(chunk)
    chunking_ms = elapsed_ms(stage_started)

    stage_started = time.perf_counter()
    vectors = embed_audio_files(chunks)
    embedding_ms = elapsed_ms(stage_started)

    stage_started = time.perf_counter()
    segments = [
        MediaSegmentResponse(
            segmentType="audio", index=index, startMs=start_ms, endMs=end_ms,
            transcript=overlapping_transcript(start_ms, end_ms, transcript_segments),
            tags=classify_audio(vector) if auto_tags else [], audioVector=vector,
        )
        for index, ((start_ms, end_ms), vector) in enumerate(zip(intervals, vectors))
    ]
    return segments, {
        "audioChunkingMs": chunking_ms,
        "audioEmbeddingMs": embedding_ms,
        "audioTaggingMs": elapsed_ms(stage_started),
    }


def merge_unique_tags(groups: list[list[str]], limit: int = 12) -> list[str]:
    result = []
    seen = set()
    for group in groups:
        for value in group:
            key = value.casefold()
            if key not in seen:
                seen.add(key)
                result.append(value)
                if len(result) >= limit:
                    return result
    return result


@app.get("/healthz")
def health() -> dict:
    return {
        "status": "ok",
        "device": DEVICE,
        "model": MODEL_NAME,
        "capabilities": {
            "photo": True,
            "video": bool(shutil.which("ffmpeg") and shutil.which("ffprobe") and WHISPER_INSTALLED and CLAP_INSTALLED),
            "audio": bool(shutil.which("ffmpeg") and shutil.which("ffprobe") and WHISPER_INSTALLED and CLAP_INSTALLED),
        },
        "dependencies": {
            "ffmpeg": bool(shutil.which("ffmpeg")),
            "ffprobe": bool(shutil.which("ffprobe")),
            "avif": features.check("avif"),
            "whisper": WHISPER_INSTALLED,
            "clap": CLAP_INSTALLED,
        },
        "derivativeImages": {
            "format": "avif",
            "quality": DERIVATIVE_IMAGE_QUALITY,
            "speed": DERIVATIVE_IMAGE_SPEED,
            "photoMaxEdge": PHOTO_THUMBNAIL_MAX_EDGE,
            "rawMaxEdge": RAW_THUMBNAIL_MAX_EDGE,
            "rawQuality": RAW_THUMBNAIL_QUALITY,
            "videoMaxEdge": VIDEO_KEYFRAME_MAX_EDGE,
            "videoThumbnailFrames": 2,
            "persistentVideoKeyframes": False,
            "audioThumbnails": False,
        },
    }


@app.post("/v1/embeddings", response_model=EmbedResponse)
def create_embeddings(request: EmbedRequest) -> EmbedResponse:
    paths = [safe_path(value) for value in request.paths]
    vectors = embed(paths)
    return EmbedResponse(
        model=f"{MODEL_NAME}/{MODEL_WEIGHTS}",
        dimensions=len(vectors[0]),
        items=[EmbeddingItem(path=str(path), vector=vector) for path, vector in zip(paths, vectors)],
    )


@app.post("/v1/analyze", response_model=AnalyzeResponse)
def analyze_photo(request: AnalyzeRequest) -> AnalyzeResponse:
    total_started = time.perf_counter()
    path = safe_path(request.path)

    stage_started = time.perf_counter()
    image = load_image(path)
    decode_ms = elapsed_ms(stage_started)

    stage_started = time.perf_counter()
    vector = embed_images([image])[0]
    embedding_ms = elapsed_ms(stage_started)

    thumbnail_resize_ms = 0.0
    thumbnail_encode_ms = 0.0
    if request.thumbnail_path:
        is_raw = path.suffix.lower() in RAW_EXTENSIONS
        max_edge = RAW_THUMBNAIL_MAX_EDGE if is_raw else PHOTO_THUMBNAIL_MAX_EDGE
        quality = RAW_THUMBNAIL_QUALITY if is_raw else DERIVATIVE_IMAGE_QUALITY
        stage_started = time.perf_counter()
        thumbnail = make_thumbnail(image, max_edge)
        thumbnail_resize_ms = elapsed_ms(stage_started)
        stage_started = time.perf_counter()
        save_derivative(thumbnail, safe_thumbnail_path(request.thumbnail_path), quality=quality)
        thumbnail_encode_ms = elapsed_ms(stage_started)

    stage_started = time.perf_counter()
    color = dominant_color(image)
    dominant_color_ms = elapsed_ms(stage_started)

    stage_started = time.perf_counter()
    tags = classify(vector)
    tagging_ms = elapsed_ms(stage_started)
    return AnalyzeResponse(
        vector=vector,
        tags=tags,
        dominantColor=color,
        timingsMs={
            "decodeMs": decode_ms,
            "embeddingMs": embedding_ms,
            "thumbnailResizeMs": thumbnail_resize_ms,
            "thumbnailEncodeMs": thumbnail_encode_ms,
            "thumbnailMs": round(thumbnail_resize_ms + thumbnail_encode_ms, 3),
            "dominantColorMs": dominant_color_ms,
            "taggingMs": tagging_ms,
            "totalMs": elapsed_ms(total_started),
        },
    )

@app.post("/v1/analyze-batch", response_model=AnalyzeBatchResponse)
def analyze_photo_batch(request: AnalyzeBatchRequest) -> AnalyzeBatchResponse:
    total_started = time.perf_counter()
    paths = [safe_path(item.path) for item in request.items]
    images = [load_image(path) for path in paths]
    vectors = embed_images(images)
    results = []
    for item, path, image, vector in zip(request.items, paths, images, vectors):
        thumbnail_started = time.perf_counter()
        if item.thumbnail_path:
            is_raw = path.suffix.lower() in RAW_EXTENSIONS
            max_edge = RAW_THUMBNAIL_MAX_EDGE if is_raw else PHOTO_THUMBNAIL_MAX_EDGE
            quality = RAW_THUMBNAIL_QUALITY if is_raw else DERIVATIVE_IMAGE_QUALITY
            save_derivative(make_thumbnail(image, max_edge), safe_thumbnail_path(item.thumbnail_path), quality=quality)
        results.append(AnalyzeBatchItemResponse(
            path=str(path), vector=vector, tags=classify(vector), dominantColor=dominant_color(image),
            timingsMs={"thumbnailMs": elapsed_ms(thumbnail_started), "batchTotalMs": elapsed_ms(total_started)},
        ))
    return AnalyzeBatchResponse(items=results)


@app.post("/v1/analyze-media", response_model=AnalyzeMediaResponse)
def analyze_media(request: AnalyzeMediaRequest) -> AnalyzeMediaResponse:
    total_started = time.perf_counter()
    path = safe_path(request.path)

    stage_started = time.perf_counter()
    probe = probe_media(path)
    description = media_description(path, probe)
    timings = {"probeMs": elapsed_ms(stage_started)}

    visual = []
    audio = []
    transcript = ""
    transcript_segments = []
    with tempfile.TemporaryDirectory(prefix="apofocus-media-") as temporary_value:
        temporary_dir = Path(temporary_value)
        if description["media_type"] == "video":
            thumbnail_path = safe_thumbnail_path(request.thumbnail_path) if request.thumbnail_path else None
            visual, visual_timings = visual_segments(
                path, description["duration_ms"], temporary_dir / "visual", thumbnail_path, request.auto_tags
            )
            timings.update(visual_timings)
            timings["thumbnailMs"] = round(
                timings.get("thumbnailComposeMs", 0.0) + timings.get("thumbnailEncodeMs", 0.0), 3
            )
        if description["has_audio"]:
            stage_started = time.perf_counter()
            normalized_audio = temporary_dir / "source.wav"
            ffmpeg_extract_audio(path, normalized_audio)
            timings["audioExtractMs"] = elapsed_ms(stage_started)

            stage_started = time.perf_counter()
            transcript, transcript_segments = transcribe_audio(normalized_audio)
            timings["transcriptionMs"] = elapsed_ms(stage_started)

            audio, audio_timings = audio_segments(
                normalized_audio, description["duration_ms"], transcript_segments, temporary_dir, request.auto_tags
            )
            timings.update(audio_timings)

    stage_started = time.perf_counter()
    automatic_tags = merge_unique_tags(
        [segment.tags for segment in visual] + [segment.tags for segment in audio]
    ) if request.auto_tags else []
    timings["tagMergeMs"] = elapsed_ms(stage_started)
    # ffprobe echoes the absolute source filename. Keep the useful technical
    # metadata without persisting or returning the machine's storage layout.
    probe.get("format", {}).pop("filename", None)
    timings["totalMs"] = elapsed_ms(total_started)
    return AnalyzeMediaResponse(
        mediaType=description["media_type"],
        durationMs=description["duration_ms"],
        mimeType=description["mime_type"],
        codec=description["codec"],
        dimensions=description["dimensions"],
        sampleRate=description["sample_rate"],
        channels=description["channels"],
        recordedAt=description["recorded_at"],
        transcript=transcript,
        tags=automatic_tags,
        metadata={
            "ffprobe": probe,
            "models": {
                "visual": f"{MODEL_NAME}/{MODEL_WEIGHTS}" if visual else "",
                "speech": f"whisper/{WHISPER_MODEL}" if description["has_audio"] else "",
                "audio": "laion-clap" if audio else "",
            },
            "derivativeImages": {
                "format": "avif",
                "videoThumbnailFrames": 2,
                "persistentVideoKeyframes": False,
                "audioThumbnails": False,
            },
        },
        segments=visual + audio,
        timingsMs=timings,
    )
