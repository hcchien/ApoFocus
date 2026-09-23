"""Optional Qwen2-VL deep photo analysis service for ApoFocus.

This process is intentionally separate from the always-on OpenCLIP service.
It accepts only allowlisted local paths and loads the large model lazily on the
first analysis request. Do not expose this service to the public internet.
"""

from __future__ import annotations

import json
import os
import re
from functools import lru_cache
from pathlib import Path

import torch
import pillow_avif  # noqa: F401 - registers AVIF support with Pillow
from PIL import Image
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from transformers import AutoModelForMultimodalLM, AutoProcessor


MODEL_ID = os.getenv("DEEP_ANALYSIS_MODEL", "Qwen/Qwen2-VL-7B-Instruct")
MODEL_REVISION = os.getenv("DEEP_ANALYSIS_MODEL_REVISION", "main")
PROMPT_VERSION = os.getenv("DEEP_ANALYSIS_PROMPT_VERSION", "apofocus-photo-v1")
MIN_PIXELS = int(os.getenv("QWEN_MIN_PIXELS", str(256 * 28 * 28)))
MAX_PIXELS = int(os.getenv("QWEN_MAX_PIXELS", str(1280 * 28 * 28)))
MAX_NEW_TOKENS = int(os.getenv("QWEN_MAX_NEW_TOKENS", "768"))
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
ALLOWED_ROOTS = tuple(dict.fromkeys((*PHOTO_ROOTS, *THUMBNAIL_ROOTS)))

SYSTEM_PROMPT = """你是專業照片檔案管理助理。只根據畫面中可以直接觀察到的內容回答；不要猜測真實人物身分、族群、宗教、健康、政治立場或其他敏感屬性。請以繁體中文輸出有效 JSON，不要使用 Markdown。格式必須是：
{"caption":"一到兩句客觀描述","objects":["物件"],"actions":["動作"],"visibleText":["可辨識文字"],"suggestedTags":["適合搜尋的短標籤"]}
若某欄沒有內容，使用空陣列。標籤至多 12 個，不要重複。"""

app = FastAPI(title="ApoFocus Deep Analysis", version="1.0.0")


class AnalyzePhotoRequest(BaseModel):
    path: str


class AnalyzePhotoResponse(BaseModel):
    model: str
    prompt_version: str = Field(alias="promptVersion")
    caption: str
    objects: list[str] = Field(default_factory=list)
    actions: list[str] = Field(default_factory=list)
    visible_text: list[str] = Field(default_factory=list, alias="visibleText")
    suggested_tags: list[str] = Field(default_factory=list, alias="suggestedTags")


def safe_path(value: str) -> Path:
    try:
        candidate = Path(value).expanduser().resolve(strict=True)
    except (OSError, RuntimeError) as error:
        raise HTTPException(status_code=400, detail="photo path does not exist") from error
    if not any(candidate.is_relative_to(root) for root in ALLOWED_ROOTS):
        raise HTTPException(status_code=403, detail="photo path is outside configured roots")
    if not candidate.is_file():
        raise HTTPException(status_code=400, detail="photo path is not a regular file")
    return candidate


@lru_cache(maxsize=1)
def load_pipeline():
    processor = AutoProcessor.from_pretrained(
        MODEL_ID,
        revision=MODEL_REVISION,
        min_pixels=MIN_PIXELS,
        max_pixels=MAX_PIXELS,
    )
    model = AutoModelForMultimodalLM.from_pretrained(
        MODEL_ID,
        revision=MODEL_REVISION,
        torch_dtype="auto",
        device_map="auto",
    )
    model.eval()
    return model, processor


def parse_json_response(value: str) -> dict:
    text = value.strip()
    fenced = re.fullmatch(r"```(?:json)?\s*(.*?)\s*```", text, flags=re.DOTALL | re.IGNORECASE)
    if fenced:
        text = fenced.group(1)
    try:
        payload = json.loads(text)
    except json.JSONDecodeError as error:
        raise HTTPException(status_code=502, detail="model did not return valid JSON") from error
    if not isinstance(payload, dict) or not isinstance(payload.get("caption"), str):
        raise HTTPException(status_code=502, detail="model response is missing a caption")
    for key in ("objects", "actions", "visibleText", "suggestedTags"):
        values = payload.get(key, [])
        if not isinstance(values, list) or any(not isinstance(item, str) for item in values):
            raise HTTPException(status_code=502, detail=f"model response field {key} is invalid")
        cleaned = []
        seen = set()
        for item in values:
            item = item.strip()
            normalized = item.casefold()
            if item and normalized not in seen:
                seen.add(normalized)
                cleaned.append(item)
        payload[key] = cleaned[:12] if key == "suggestedTags" else cleaned
    payload["caption"] = payload["caption"].strip()
    return payload


def analyze(path: Path) -> dict:
    model, processor = load_pipeline()
    with Image.open(path) as image:
        rgb_image = image.convert("RGB")
    messages = [
        {"role": "system", "content": [{"type": "text", "text": SYSTEM_PROMPT}]},
        {
            "role": "user",
            "content": [
                {"type": "image", "image": rgb_image},
                {"type": "text", "text": "請分析這張照片並依指定格式輸出。"},
            ],
        },
    ]
    inputs = processor.apply_chat_template(
        messages,
        add_generation_prompt=True,
        tokenize=True,
        return_dict=True,
        return_tensors="pt",
    ).to(model.device)
    with torch.inference_mode():
        generated = model.generate(**inputs, max_new_tokens=MAX_NEW_TOKENS, do_sample=False)
    prompt_length = inputs["input_ids"].shape[-1]
    response = processor.decode(generated[0][prompt_length:], skip_special_tokens=True)
    return parse_json_response(response)


@app.get("/healthz")
def health() -> dict:
    return {
        "status": "ok",
        "model": MODEL_ID,
        "promptVersion": PROMPT_VERSION,
        "modelLoaded": load_pipeline.cache_info().currsize > 0,
    }


@app.post("/v1/analyze-photo", response_model=AnalyzePhotoResponse)
def analyze_photo(request: AnalyzePhotoRequest) -> AnalyzePhotoResponse:
    payload = analyze(safe_path(request.path))
    return AnalyzePhotoResponse(
        model=MODEL_ID,
        promptVersion=PROMPT_VERSION,
        caption=payload["caption"],
        objects=payload["objects"],
        actions=payload["actions"],
        visibleText=payload["visibleText"],
        suggestedTags=payload["suggestedTags"],
    )
