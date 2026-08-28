#!/usr/bin/env python3
"""Generate ApoFocus LaunchAgent plists without fragile XML escaping."""

from __future__ import annotations

import argparse
import json
import os
import plistlib
from pathlib import Path


def write_agent(
    destination: Path,
    label: str,
    arguments: list[str],
    environment: dict[str, str],
    working_directory: Path,
    stdout_path: Path,
    stderr_path: Path,
    *,
    run_at_load: bool = True,
    keep_alive: bool = True,
    start_calendar_interval: dict[str, int] | None = None,
) -> None:
    payload = {
        "Label": label,
        "ProgramArguments": arguments,
        "EnvironmentVariables": environment,
        "WorkingDirectory": str(working_directory),
        "RunAtLoad": run_at_load,
        "KeepAlive": keep_alive,
        "ProcessType": "Background",
        "ThrottleInterval": 5,
        "StandardOutPath": str(stdout_path),
        "StandardErrorPath": str(stderr_path),
    }
    if start_calendar_interval is not None:
        payload["StartCalendarInterval"] = start_calendar_interval
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("wb") as handle:
        plistlib.dump(payload, handle, fmt=plistlib.FMT_XML, sort_keys=False)
    os.chmod(destination, 0o600)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--state-dir", type=Path, required=True)
    parser.add_argument("--logs-dir", type=Path, required=True)
    parser.add_argument("--postgres-bin", type=Path, required=True)
    parser.add_argument("--postgres-data", type=Path, required=True)
    parser.add_argument("--postgres-port", required=True)
    parser.add_argument("--app-bin", type=Path, required=True)
    parser.add_argument("--worker-bin", type=Path, required=True)
    parser.add_argument("--mcp-bin", type=Path, required=True)
    parser.add_argument("--backup-bin", type=Path, required=True)
    parser.add_argument("--python-bin", type=Path, required=True)
    parser.add_argument("--embedding-dir", type=Path, required=True)
    parser.add_argument("--database-url", required=True)
    parser.add_argument("--addr", required=True)
    parser.add_argument("--app-url", required=True)
    parser.add_argument("--library-root", type=Path, required=True)
    parser.add_argument("--import-roots", required=True)
    parser.add_argument("--brew-prefix", type=Path, required=True)
    parser.add_argument("--backup-root", default="")
    parser.add_argument("--backup-status", default="")
    parser.add_argument("--backup-volume-uuid", default="")
    args = parser.parse_args()

    path_value = f"{args.postgres_bin}:{args.brew_prefix / 'bin'}:/usr/bin:/bin:/usr/sbin:/sbin"
    common_environment = {
        "HOME": str(Path.home()),
        "PATH": path_value,
        "LANG": "en_US.UTF-8",
        "LC_ALL": "en_US.UTF-8",
    }
    write_agent(
        args.output_dir / "com.apofocus.postgres.plist",
        "com.apofocus.postgres",
        [
            str(args.postgres_bin / "postgres"),
            "-D",
            str(args.postgres_data),
            "-p",
            args.postgres_port,
            "-c",
            "listen_addresses=127.0.0.1",
        ],
        common_environment,
        args.state_dir,
        args.logs_dir / "postgres.log",
        args.logs_dir / "postgres.error.log",
    )

    backup_environment = {
        **common_environment,
        "DATABASE_URL": args.database_url,
        "POSTGRES_BIN": str(args.postgres_bin),
        "POSTGRES_DATA": str(args.postgres_data),
        "APOFOCUS_BACKUP_ROOT": args.backup_root,
        "APOFOCUS_BACKUP_STATUS": args.backup_status,
        "APOFOCUS_BACKUP_VOLUME_UUID": args.backup_volume_uuid,
    }
    if args.backup_root:
        write_agent(
            args.output_dir / "com.apofocus.backup.plist",
            "com.apofocus.backup",
            [str(args.backup_bin), "scheduled"],
            backup_environment,
            args.state_dir,
            args.logs_dir / "backup.log",
            args.logs_dir / "backup.error.log",
            run_at_load=True,
            keep_alive=False,
            start_calendar_interval={"Hour": 3, "Minute": 0},
        )
        write_agent(
            args.output_dir / "com.apofocus.backup-verify.plist",
            "com.apofocus.backup-verify",
            [str(args.backup_bin), "verify"],
            backup_environment,
            args.state_dir,
            args.logs_dir / "backup-verify.log",
            args.logs_dir / "backup-verify.error.log",
            run_at_load=False,
            keep_alive=False,
            start_calendar_interval={"Day": 1, "Hour": 4, "Minute": 0},
        )
    else:
        (args.output_dir / "com.apofocus.backup.plist").unlink(missing_ok=True)
        (args.output_dir / "com.apofocus.backup-verify.plist").unlink(missing_ok=True)

    model_cache = args.state_dir / "models"
    photo_roots = args.import_roots.split(os.pathsep)
    if str(args.library_root) not in photo_roots:
        photo_roots.append(str(args.library_root))
    embedding_environment = {
        **common_environment,
        "PHOTO_ROOTS": os.pathsep.join(photo_roots),
        "THUMBNAIL_ROOTS": str(args.library_root),
        "PHOTO_LIBRARY_ROOT": str(args.library_root),
        "HF_HOME": str(model_cache / "huggingface"),
        "TORCH_HOME": str(model_cache / "torch"),
        "XDG_CACHE_HOME": str(model_cache / "xdg"),
        "WHISPER_DOWNLOAD_ROOT": str(model_cache / "whisper"),
    }
    write_agent(
        args.output_dir / "com.apofocus.embedding.plist",
        "com.apofocus.embedding",
        [
            str(args.python_bin),
            "-m",
            "uvicorn",
            "app:app",
            "--app-dir",
            str(args.embedding_dir),
            "--host",
            "127.0.0.1",
            "--port",
            "8090",
        ],
        embedding_environment,
        args.embedding_dir,
        args.logs_dir / "embedding.log",
        args.logs_dir / "embedding.error.log",
    )

    web_environment = {
        **common_environment,
        "ADDR": args.addr,
        "DATABASE_URL": args.database_url,
        "PHOTO_LIBRARY_ROOT": str(args.library_root),
        "APOFOCUS_IMPORT_ROOTS": args.import_roots,
        "EMBEDDING_SERVICE_URL": "http://127.0.0.1:8090",
        "THUMBNAIL_ROOTS": str(args.library_root),
        "APOFOCUS_APP_URL": args.app_url,
    }
    write_agent(
        args.output_dir / "com.apofocus.web.plist",
        "com.apofocus.web",
        [str(args.app_bin)],
        web_environment,
        args.state_dir,
        args.logs_dir / "web.log",
        args.logs_dir / "web.error.log",
    )

    worker_environment = {
        **common_environment,
        "DATABASE_URL": args.database_url,
        "PHOTO_LIBRARY_ROOT": str(args.library_root),
        "APOFOCUS_IMPORT_ROOTS": args.import_roots,
        "EMBEDDING_SERVICE_URL": "http://127.0.0.1:8090",
        "APOFOCUS_CATALOG_WORKERS": "2",
    }
    write_agent(
        args.output_dir / "com.apofocus.worker.plist",
        "com.apofocus.worker",
        [str(args.worker_bin)],
        worker_environment,
        args.state_dir,
        args.logs_dir / "worker.log",
        args.logs_dir / "worker.error.log",
    )

    mcp_config = {
        "mcpServers": {
            "apofocus": {
                "command": str(args.mcp_bin),
                "env": {
                    "DATABASE_URL": args.database_url,
                    "PHOTO_LIBRARY_ROOT": str(args.library_root),
                    "APOFOCUS_IMPORT_ROOTS": args.import_roots,
                    "EMBEDDING_SERVICE_URL": "http://127.0.0.1:8090",
                    "APOFOCUS_APP_URL": args.app_url,
                    "APOFOCUS_BACKUP_ROOT": args.backup_root,
                    "APOFOCUS_BACKUP_STATUS": args.backup_status,
                    "APOFOCUS_BACKUP_VOLUME_UUID": args.backup_volume_uuid,
                },
            }
        }
    }
    args.state_dir.mkdir(parents=True, exist_ok=True)
    mcp_path = args.state_dir / "mcp-server.json"
    with mcp_path.open("w", encoding="utf-8") as handle:
        json.dump(mcp_config, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    os.chmod(mcp_path, 0o600)


if __name__ == "__main__":
    main()
