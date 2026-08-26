# ApoFocus

ApoFocus 是為職業攝影師設計的照片、影片與音訊索引網站。Go 負責 API、網頁、MCP server 與持久化 batch queue，PostgreSQL 負責結構化資料、全文檢索與篩選，pgvector 負責片段向量搜尋；本機 Python service 以 OpenCLIP、Whisper、CLAP 與 FFmpeg 產生縮圖、keyframes、逐字稿、embedding 與自動 Tags。

## 已完成的功能

- 依拍照年份、專題、多個 Tags、相機、鏡頭、ISO 範圍與 GeoTag 篩選
- 搜尋標題、器材與地點
- 照片詳情與完整 EXIF 摘要
- 以選定照片做 cosine similarity 向量搜尋
- 桌面與手機版照片牆、篩選抽屜及相似照片介面
- 繁體中文、英文、德文介面，依瀏覽器語言自動選擇並記住使用者設定
- Photos、Videos、Audios 分離的頂層 tabs，各自連接搜尋、facets、播放器與詳情 API
- 影片固定間隔 keyframe + OpenCLIP 視覺向量；影片音軌及純音訊以 Whisper 建逐字稿、CLAP 建聲音向量
- 影片／音訊支援 codec、長度、是否有逐字稿、專題、年份及 Tags 篩選
- 無資料庫時自動使用內建展示資料
- PostgreSQL schema、索引與 512 維 HNSW pgvector index
- OpenCLIP 批次向量 worker，以及可獨立呼叫的本機 embedding API
- MCP 照片匯入：先預覽 EXIF／建議分類，再安全複製至 managed library 並寫入 DB
- 以 SHA-256 去重；重試同一張照片不會產生重複資料
- Finder 式虛擬資料夾：照片依年份、專題、Tags、相機與鏡頭；影片／音訊另可依 Codec 瀏覽
- 手動 Collections 與保存 filter JSON 的智慧資料夾
- 外接硬碟／整個 Volume 批次整理、SSE 即時進度、逐張錯誤與中止功能
- Managed library filesystem watcher：檔案改名或在 library 內搬動時，自動修正 DB path；遺失與磁碟離線狀態會顯示在 UI
- macOS 0-to-1 installer：Homebrew dependencies、專用 PostgreSQL/pgvector、Python models、Go binaries 與 LaunchAgents

## 系統邊界

```text
LLM / MCP host ──stdio──> Go MCP server ──transaction──> PostgreSQL + pgvector
                              │                         ▲
                              ├──copy──> Local library  │
                              └──HTTP──> Python models + FFmpeg┘

Browser ──HTTP──> Go Web API ──SQL──> PostgreSQL + pgvector
```

`photos` 與 `media_assets` 的 `path`／`thumbnail_path` 保存伺服器或 NAS 上的實際路徑，只供後端、MCP server 與向量 worker 使用。瀏覽器只取得 `image_url`／`media_url` 與 `thumbnail_url`；API 不輸出本機 path，保存的 ffprobe metadata 也會移除來源 filename，避免洩漏儲存拓撲。

## macOS 從零安裝

在 repository 目錄執行一個 script 即可：

```bash
bash scripts/install_macos.sh
```

Installer 可重複執行，會完成：

1. 檢查 Xcode Command Line Tools；若尚未安裝，開啟 macOS 官方安裝流程並提示完成後重跑。
2. 安裝 Homebrew（若需要），以及 Go、Python 3.12、PostgreSQL 18、pgvector、FFmpeg、LibRaw、libsndfile 與模型編譯依賴。
3. 在 `~/Library/Application Support/ApoFocus` 建立獨立 PostgreSQL cluster、隨機本機密碼、Python venv、model cache 與 Go binaries，不會使用或修改既有 PostgreSQL database。
4. 依目前 schema 狀態安全套用尚未安裝的 migrations。
5. 預先下載並載入驗證 OpenCLIP、Whisper base 與 LAION-CLAP。
6. 安裝 `com.apofocus.postgres`、`com.apofocus.embedding`、`com.apofocus.web` 三個 LaunchAgents，登入後自動啟動並在失敗時重啟。
7. 驗證 PostgreSQL/pgvector、embedding health endpoint 與 Web API。

預設位置：

| 項目 | 路徑 |
|---|---|
| Managed library | `~/Pictures/ApoFocus Library` |
| MCP / batch inbox | `~/Pictures/ApoFocus Inbox` |
| App、DB、venv、models | `~/Library/Application Support/ApoFocus` |
| Logs | `~/Library/Logs/ApoFocus` |
| MCP client 設定範本 | `~/Library/Application Support/ApoFocus/mcp-server.json` |
| Web | `http://127.0.0.1:8080` |

若 library 要直接放在已掛載的外接硬碟：

```bash
bash scripts/install_macos.sh \
  --library-root "/Volumes/PHOTO_DISK/ApoFocus Library"
```

其他常用選項：

```bash
bash scripts/install_macos.sh --check-only
bash scripts/install_macos.sh --skip-model-download
bash scripts/install_macos.sh --no-start
bash scripts/install_macos.sh --postgres-port 55433
```

完整安裝需要下載數 GB 的 Python packages 與模型。Script 僅讓 Web 監聽 `127.0.0.1`；目前系統沒有遠端登入驗證，因此 installer 不接受 public listen address。

安裝後可用控制工具：

```bash
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" doctor
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" status
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" restart
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" logs
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" open
```

外接硬碟、`Pictures` 或其他受保護資料夾第一次讀取時，macOS 可能要求 Files and Folders 權限。若 LaunchAgent 被拒絕，請到 System Settings → Privacy & Security 授權，再執行 `apofocusctl restart`。

## 先看介面（不需要資料庫）

需求：Go 1.26+。

```bash
go run ./cmd/apofocus
```

打開 [http://localhost:8080](http://localhost:8080)。未設定 `DATABASE_URL` 時會自動載入 12 張展示資料，所有篩選與相似搜尋都可操作。

## PostgreSQL / pgvector

若電腦有 Docker：

```bash
docker compose up -d postgres
export DATABASE_URL='postgres://apofocus:apofocus@localhost:5432/apofocus?sslmode=disable'
make migrate-up
make run
```

也可以使用既有 PostgreSQL，只要先安裝 `vector` extension，再執行 [migrations/000001_init.sql](migrations/000001_init.sql)。

資料模型的重要欄位：

| 欄位 | 用途 |
|---|---|
| `path` | 原始照片的目前絕對路徑或儲存系統 canonical path，唯一且不對前端公開 |
| `thumbnail_path` | 本機縮圖路徑，可為空 |
| `storage_root_id` / `relative_path` | 指向 managed library root，並保存可隨掛載點重建的相對路徑 |
| `file_id` / `thumbnail_file_id` | filesystem device + inode 識別碼，用於在 rename/move event 後辨識同一檔案 |
| `availability_status` / `thumbnail_status` | `available`、`missing`、`volume_offline` 或 `unknown` |
| `image_url` / `thumbnail_url` | 可交付瀏覽器的 URL，建議是短效簽名 URL 或 Go 的受權限保護 route |
| `metadata jsonb` | 未提升為常用欄位的 EXIF、IPTC、XMP 等延伸資訊 |
| `embedding vector(512)` | 經 L2 normalization 的 OpenCLIP 影像向量 |

影片與音訊存放於 `media_assets`，可搜尋的時間片段存放於 `media_segments`：影片片段可同時有 `keyframe_path` 與 `visual_embedding vector(512)`；有音軌的影片及純音訊片段使用 `audio_embedding vector(512)`。逐字稿保留在 asset 與片段層，方便全文搜尋和播放器時間軸顯示。

## 本機產生照片向量

Python 適合這一層，因為 PyTorch、OpenCLIP 與 GPU/MPS 支援都比 Go 成熟。預設模型 `ViT-B-32/laion2b_s34b_b79k` 的輸出正好是 512 維，與 migration 一致。

```bash
make embedding-install
export DATABASE_URL='postgres://apofocus:apofocus@localhost:5432/apofocus?sslmode=disable'
export PHOTO_ROOTS='/Volumes/PhotoArchive:/Volumes/PhotoBackup'
make embedding-index
```

worker 只讀取 `embedding IS NULL` 的照片，按批次載入 `path`、正規化向量後寫回 PostgreSQL。`PHOTO_ROOTS` 是必要的路徑 allowlist，避免任意路徑被影像服務讀取。

若匯入流程想即時取得向量，也可啟動只監聽本機的 API：

```bash
make embedding-serve
curl -X POST http://127.0.0.1:8090/v1/embeddings \
  -H 'Content-Type: application/json' \
  -d '{"paths":["/Volumes/PhotoArchive/2026/example.jpg"]}'
```

第一次使用 OpenCLIP 會下載模型權重。Apple Silicon 會優先使用 MPS，NVIDIA 環境使用 CUDA，其他環境則回退到 CPU。

常見相機 RAW 格式會在 Pillow 無法解碼時改由 LibRaw／`rawpy` 產生預覽，包括 DNG、ARW、CR2、CR3、NEF 與 RAF。原始 RAW 檔只會被複製，不會重新編碼。

## 本機影片與音訊辨識

影片與音訊沿用同一個 `embedding-serve` process，且 batch worker 仍一次只處理一個檔案：

- Video：FFmpeg 每 10 秒取一個 keyframe（上限 300），OpenCLIP 為每個 frame 產生 512 維視覺向量。
- Video audio / Audio：FFmpeg 正規化為 mono 48 kHz WAV，Whisper `base` 產生逐字稿與 timestamp。
- Audio segments：每 30 秒切一段（上限 600），CLAP 為每段產生 512 維聲音向量與聲音類型 Tags。
- Audio thumbnail：產生靜態 waveform JPG；Video thumbnail：使用第一個 keyframe。

需求是 Python 3.12、FFmpeg／ffprobe；模型會在第一次使用時下載，CLAP 權重較大，請預留約 3 GB 的模型快取空間。完成第一次下載後可用完全離線模式啟動：

```bash
make embedding-install
make embedding-serve          # 第一次需連網下載模型
make embedding-serve-offline  # 權重已快取後，不做網路檢查
```

可調整 `WHISPER_MODEL`、`WHISPER_LANGUAGE`、`VIDEO_SAMPLE_SECONDS`、`MAX_VIDEO_SEGMENTS`、`AUDIO_SEGMENT_SECONDS` 與 `MAX_AUDIO_SEGMENTS`。長影片不會卡住 HTTP request：瀏覽器只建立 batch job，真正的 Python 分析在 background worker 執行，進度由 PostgreSQL 保存並透過 SSE 顯示。

## Finder 式資料夾

網站左側的「資料夾瀏覽」會開啟三欄式瀏覽器。照片以年份、專題、Tags、相機和鏡頭建立虛擬資料夾；影片與音訊則以年份、專題、Tags 和 Codec 建立。這些都是 PostgreSQL 即時產生的視圖；同一份媒體可以同時出現在多個位置，而實體檔案仍只有一份。

另外提供：

- `manual` collection：透過 `collection_photos` 保存人工選取的照片。
- `smart` collection：在 `collections.filter` 保存查詢條件，不複製照片。
- 階層：`collections.parent_id` 可以建立 Finder 式子資料夾。

相關 API：

- `GET /api/v1/folders`
- `POST /api/v1/collections`
- `POST /api/v1/collections/{id}/photos`
- `GET /api/v1/collections/{id}/photos`

## 外接硬碟批次整理

設定 `APOFOCUS_IMPORT_ROOTS=/Volumes` 可以讓使用者選擇已掛載的外接硬碟或其中一個資料夾；若希望範圍更窄，可直接指定 `/Volumes/PHOTO_DISK`。server 仍會解析 symlink 並確認選取路徑位於 allowlist 內。

批次處理使用同一個 Go process 裡的單一 background worker，照片、影片與音訊會依序處理，不會同時跑多個模型 inference。流程如下：

1. `POST /api/v1/batch-jobs` 只建立持久化工作並立即回傳 `202 Accepted`。
2. worker 依 job 的 `mediaTypes` 掃描照片、影片或音訊，略過隱藏系統資料夾，把每個檔案記錄到 `batch_items`。
3. 每個檔案依序執行 metadata、copy、縮圖／keyframes、模型辨識、Tags 與 DB transaction。
4. 網頁透過 `GET /api/v1/batch-jobs/{id}/events` 的 SSE 接收即時進度。
5. SSE 斷線時改用短輪詢；重新整理頁面不會中止工作。

這裡不使用 WebSocket，因為進度是 server 到瀏覽器的單向事件；取消使用獨立的 `POST /cancel` 即可。PostgreSQL job queue 讓 app 意外退出後能從未完成的 item 恢復，並利用媒體 SHA-256 保證重試安全。

網站內可直接開啟「批次匯入」，也可以使用 CLI。CLI 的每次 HTTP request 都很短，不會等待整批完成：

```bash
go run ./cmd/apofocus-batch \
  --source /Volumes/PHOTO_DISK/2026 \
  --media video \
  --project "島嶼日常" \
  --tag 紀實 \
  --tag 客戶精選
```

加入 `--wait=false` 可只建立工作並立即退出。預設會每秒查詢一次狀態，在 terminal 顯示完成百分比、成功與失敗數。

`--media` 可重複指定 `photo`、`video`、`audio`；完全省略時會處理三者。網站會依目前所在的 Photos／Videos／Audios tab 自動建立對應類型的 job。

Batch API：

- `POST /api/v1/batch-jobs`
- `GET /api/v1/batch-jobs/{id}`
- `GET /api/v1/batch-jobs/{id}/items`
- `GET /api/v1/batch-jobs/{id}/events` — SSE
- `POST /api/v1/batch-jobs/{id}/cancel`

## 檔案搬移與 filesystem events

設定 `DATABASE_URL` 與 `PHOTO_LIBRARY_ROOT` 後，Web app 與 MCP server 會把這個路徑登記在 `storage_roots`，啟動時確認 DB 已知的原檔、縮圖與 keyframe，接著遞迴監聽 library 內的 filesystem events。

- 檔案在同一個 filesystem 內改名或搬到 library 的其他資料夾時，device + inode 不變；watcher 會用 `file_id` 找到原資料並更新 `path`、`relative_path` 與瀏覽器 URL。
- 刪除或移出 library 時，不刪 DB row，而是標成 `missing`；外接硬碟拔除時整個 root 標成 `volume_offline`。若 app 持續執行，重新掛載後會自動恢復監聽並核對已知路徑。
- 新的匯入在寫入 DB 時就保存原檔、縮圖與 keyframe 的 file ID，不需要等下一次 scan。
- 啟動時只 `stat` 資料庫已知的路徑，不會掃整個 Volume 計算 SHA-256。只有 filesystem event 指向一個新搬入的資料夾時，才會走訪該子樹以接上其中已知 file ID。

SHA-256 仍用於內容去重；它不能在沒有 index 或 filesystem event 的情況下告訴系統「檔案搬到哪裡」。如果程式關閉期間把檔案搬出 managed library、跨 filesystem 複製後刪除，inode 也會改變，系統只能先標成 `missing`。這種情況之後可再加一個由使用者指定範圍的 relink job，以 size/mtime 縮小候選後才計算 SHA-256，而不是無條件掃所有磁碟。

## MCP：讓 LLM 匯入照片

MCP server 使用官方 Go SDK 與 stdio transport，提供三個 tools：

| Tool | 行為 |
|---|---|
| `get_photo_import_policy` | 唯讀；取得 allowlist 與 folder 規則 |
| `inspect_photo` | 唯讀；解析 EXIF／GeoTag、預覽目標 folder，可用 OpenCLIP 建議 Tags |
| `import_photo` | 寫入；`confirmed: true` 後複製原檔、產縮圖和向量，以 transaction 寫入照片、專題與 Tags |

資料夾規則為：

```text
<PHOTO_LIBRARY_ROOT>/originals/YYYY/<project>/YYYY-MM-DD_<filename>_<sha-prefix>.<ext>
<PHOTO_LIBRARY_ROOT>/thumbnails/YYYY/<project>/YYYY-MM-DD_<filename>_<sha-prefix>.jpg
```

MCP host 必須先將聊天中附加的照片落到 `APOFOCUS_IMPORT_ROOTS` 其中一個本機目錄，再把該絕對路徑傳入 `source_path`。server 會解析 symlink 並重新檢查範圍，不接受任意 filesystem path。來源檔一律保留，匯入採 copy。

啟動順序：

```bash
export DATABASE_URL='postgres://apofocus:apofocus@localhost:5432/apofocus?sslmode=disable'
export APOFOCUS_IMPORT_ROOTS='/Users/me/Downloads/ApoFocus-Inbox'
export PHOTO_LIBRARY_ROOT='/Volumes/PhotoArchive/ApoFocus'
export PHOTO_ROOTS="$APOFOCUS_IMPORT_ROOTS"
export THUMBNAIL_ROOTS="$PHOTO_LIBRARY_ROOT"
export EMBEDDING_SERVICE_URL='http://127.0.0.1:8090'

make embedding-serve   # terminal 1
make build-mcp         # 產生 bin/apofocus-mcp
```

通用的 MCP client 設定如下；實際最外層設定名稱依 client 而異：

```json
{
  "mcpServers": {
    "apofocus": {
      "command": "/absolute/path/to/ApoFocus/bin/apofocus-mcp",
      "env": {
        "DATABASE_URL": "postgres://apofocus:apofocus@localhost:5432/apofocus?sslmode=disable",
        "APOFOCUS_IMPORT_ROOTS": "/Users/me/Downloads/ApoFocus-Inbox",
        "PHOTO_LIBRARY_ROOT": "/Volumes/PhotoArchive/ApoFocus",
        "EMBEDDING_SERVICE_URL": "http://127.0.0.1:8090"
      }
    }
  }
}
```

建議的 LLM 操作順序：

1. 使用者附加照片，MCP host 將檔案放入 allowlisted inbox。
2. LLM 呼叫 `inspect_photo`，把 EXIF、建議 folder 與 Tags 呈現給使用者。
3. 使用者確認或修改標題、專題、地點與 Tags。
4. LLM 呼叫 `import_photo` 並傳入 `confirmed: true`。
5. 回傳 DB photo ID、實際原圖／縮圖 path、最終 Tags 與向量維度。

自動 Tags 完全在本機執行：OpenCLIP 會從可在 [services/embedding/app.py](services/embedding/app.py) 調整的攝影詞彙中挑出最多四個高於信心門檻的標籤；數量與門檻可透過 `AUTO_TAG_LIMIT`、`AUTO_TAG_MIN_SCORE` 調整。LLM 提供的 Tags 會與自動 Tags 合併、去重，而非直接覆蓋。

## API

- `GET /api/v1/photos` — 支援 `q`、`year`、`project`、重複的 `tag`、`camera`、`lens`、`min_iso`、`max_iso`、`has_location`
- `GET /api/v1/photos/{id}` — 照片詳情
- `GET /api/v1/photos/{id}/similar?limit=6` — pgvector cosine similarity
- `GET /api/v1/facets` — 各篩選條件與數量
- `GET /api/v1/videos`、`GET /api/v1/audios` — 支援 `q`、`year`、`project`、`tag`、`codec`、`min_duration_ms`、`max_duration_ms`、`has_transcript`
- `GET /api/v1/videos/{id}`、`GET /api/v1/audios/{id}` — player、逐字稿與索引片段資料
- `GET /api/v1/videos/{id}/similar?modality=visual|audio` — 影片可依 keyframe 或聲音距離搜尋
- `GET /api/v1/audios/{id}/similar?modality=audio` — CLAP 聲音距離搜尋
- `GET /api/v1/videos/facets`、`GET /api/v1/audios/facets` — 媒體 facets
- `GET /healthz` — Go service health check

## 驗證

```bash
go test ./...
go vet ./...
node --check web/static/app.js
python3 -m py_compile services/embedding/app.py services/embedding/worker.py
```
