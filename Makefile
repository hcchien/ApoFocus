.PHONY: run test build-mcp build-batch run-mcp migrate-up migrate-down embedding-install embedding-serve embedding-serve-offline embedding-index install-macos

install-macos:
	bash scripts/install_macos.sh

run:
	go run ./cmd/apofocus

test:
	go test ./...

build-mcp:
	mkdir -p bin
	go build -o bin/apofocus-mcp ./cmd/apofocus-mcp

build-batch:
	mkdir -p bin
	go build -o bin/apofocus-batch ./cmd/apofocus-batch

run-mcp:
	go run ./cmd/apofocus-mcp

migrate-up:
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f migrations/000001_init.sql -f migrations/000002_ingest.sql -f migrations/000003_folders_and_batch.sql -f migrations/000004_multimedia.sql -f migrations/000005_storage_tracking.sql

migrate-down:
	psql "$(DATABASE_URL)" -v ON_ERROR_STOP=1 -f migrations/000005_down.sql -f migrations/000004_down.sql -f migrations/000003_down.sql -f migrations/000001_down.sql

embedding-install:
	python3 -m venv .venv
	.venv/bin/pip install -r services/embedding/requirements.txt

embedding-serve:
	.venv/bin/uvicorn app:app --app-dir services/embedding --host 127.0.0.1 --port 8090

embedding-serve-offline:
	HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 .venv/bin/uvicorn app:app --app-dir services/embedding --host 127.0.0.1 --port 8090

embedding-index:
	.venv/bin/python services/embedding/worker.py
