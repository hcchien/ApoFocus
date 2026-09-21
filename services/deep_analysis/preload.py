"""Download and validate the configured Qwen model without running inference."""

from app import MODEL_ID, MODEL_REVISION, load_pipeline


def main() -> None:
    load_pipeline()
    print(f"ready: {MODEL_ID}@{MODEL_REVISION}")


if __name__ == "__main__":
    main()
