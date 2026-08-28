"""Download and validate every local model used by ApoFocus."""

from __future__ import annotations

import gc

from PIL import features

from app import (
    DEVICE,
    MODEL_NAME,
    MODEL_WEIGHTS,
    WHISPER_DEVICE,
    WHISPER_MODEL,
    clap_tag_features,
    load_clap_model,
    load_model,
    load_whisper_model,
    tag_features,
)


def release(*cached_functions) -> None:
    for function in cached_functions:
        function.cache_clear()
    gc.collect()


def main() -> None:
    if not features.check("avif"):
        raise RuntimeError("Pillow AVIF support is required")
    print("[images] Pillow AVIF encoder is ready", flush=True)

    print(f"[models] OpenCLIP {MODEL_NAME}/{MODEL_WEIGHTS} on {DEVICE}", flush=True)
    load_model()
    tag_features()
    release(tag_features, load_model)

    print(f"[models] Whisper {WHISPER_MODEL} on {WHISPER_DEVICE}", flush=True)
    load_whisper_model()
    release(load_whisper_model)

    print("[models] LAION-CLAP and audio tag features", flush=True)
    load_clap_model()
    clap_tag_features()
    release(clap_tag_features, load_clap_model)
    print("[models] all model weights are ready", flush=True)


if __name__ == "__main__":
    main()
