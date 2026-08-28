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
- 完整 MCP 操作面：照片／影音匯入、catalog 搜尋、向量相似搜尋、folders／collections，以及可監控、取消、恢復的 batch jobs
- 以 SHA-256 去重；重試同一張照片不會產生重複資料
- Finder 式虛擬資料夾：照片依年份、專題、Tags、相機與鏡頭；影片／音訊另可依 Codec 瀏覽
- 手動 Collections 與保存 filter JSON 的智慧資料夾
- 外接硬碟／整個 Volume 批次整理、SSE 即時進度、逐張錯誤與中止功能
- Managed library filesystem watcher：檔案改名或在 library 內搬動時，自動修正 DB path；遺失與磁碟離線狀態會顯示在 UI
- macOS 0-to-1 installer：Homebrew dependencies、專用 PostgreSQL/pgvector、Python models、Go binaries 與 LaunchAgents
- 外接碟 PostgreSQL 自動備份：Volume UUID 防呆、每日壓縮備份、7 日／4 週／6 月保留政策、每月實際還原測試與 MCP 維運

## 系統邊界

```text
Remote user ──LLM client remote──> Local LLM / MCP host ──stdio──> Go MCP server
                                                                    │
                                      ┌─────────────────────────────┼──────────────────────┐
                                      ▼                             ▼                      ▼
                              PostgreSQL + pgvector          Local library       Python models + FFmpeg

Browser ──HTTP──> Go Web API ──SQL──> PostgreSQL + pgvector
```

遠端連線、帳號驗證與 task session 由使用者選擇的 LLM client 負責；ApoFocus MCP 仍只在 Mac 本機透過 stdio 執行，不開 public MCP port。Agent 只在使用者傳訊息要求查看或修復時呼叫 MCP，Batch Worker 不依賴 Agent 在線。

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
6. 安裝 `com.apofocus.postgres`、`com.apofocus.embedding`、`com.apofocus.web` 三個常駐 LaunchAgents，登入後自動啟動並在失敗時重啟。
7. 若提供 `--backup-root`，另安裝每日備份與每月還原驗證兩個非持續常駐 LaunchAgents，並在安裝後背景執行第一次備份與還原測試。
8. 驗證 PostgreSQL/pgvector、embedding health endpoint 與 Web API。

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

若要把 PostgreSQL 備份放在外接 APFS Volume：

```bash
bash scripts/install_macos.sh \
  --backup-root "/Volumes/ApoFocusBackup/PostgreSQL"
```

`--backup-root` 必須位於 `/Volumes` 下。Installer 會記住最外層 Volume UUID；排程執行時若硬碟未掛載、掛載成同名的另一顆硬碟，或只剩內建碟上的同名 mount point，會拒絕寫入。若同一顆外接裝置也作為 Time Machine 目的地，請建立獨立的 APFS Volume 給 ApoFocus，不要直接把 dump 寫進 Time Machine Volume。

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
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" backup
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" verify-backup
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" logs
"$HOME/Library/Application Support/ApoFocus/bin/apofocusctl" open
```

外接硬碟、`Pictures` 或其他受保護資料夾第一次讀取時，macOS 可能要求 Files and Folders 權限。若 LaunchAgent 被拒絕，請到 System Settings → Privacy & Security 授權，再執行 `apofocusctl restart`。

## PostgreSQL 自動備份與還原驗證

啟用 `--backup-root` 後會建立兩個固定、不可注入任意命令或路徑的排程：

- `com.apofocus.backup`：每天 03:00 執行；第一次 bootstrap 也會立即開始。先檢查外接 Volume UUID 與空間，再以 `pg_dump` custom format＋zstd level 6 寫入 `.partial`；`pg_restore --list` 通過後才原子改名為 `.dump`。
- `com.apofocus.backup-verify`：每月 1 日 04:00 執行。把最新 `.dump` 完整還原到 `apofocus_verify_*` 暫存 database，確認 `photos`、`projects`、`tags` 存在後刪除暫存 database，永遠不會覆蓋 live `apofocus` database。

每日排程若從未成功驗證過，或上次驗證已超過 30 日，也會在完成新備份後接著執行還原測試。兩種 operation 共用 process lock，不會同時操作；MCP 觸發是非同步的，不會讓 LLM request 等待大型 database dump 或 restore。

還原測試會暫時在 PostgreSQL data volume 建立另一份 database，因此開始前要求內建碟至少保有「目前 DB 大小的 1.5 倍＋5GB」可用空間；不足時只記錄驗證失敗，不會冒險塞滿系統碟。若 process 被中止，下次驗證會先清除名稱符合 `apofocus_verify_*` 的殘留測試 database；超過 24 小時的 `.partial` 也會安全清理。

保留政策只在新 archive 已完成且驗證格式後才執行：最近 7 日每天一份、接著 4 週每週一份、再接著 6 個月每月一份。備份失敗、磁碟離線或空間不足時不會刪除既有備份。狀態保存在 `~/Library/Application Support/ApoFocus/backup-status.json`，logs 位於 `~/Library/Logs/ApoFocus/backup*.log`。

外接備份碟必須與運作中的內建 PostgreSQL SSD 分開。Time Machine 可以繼續備份 Mac，但不能取代 database-aware dump 與 restore test；若要防範外接備份碟本身故障，仍應定期把備份複製到第二顆輪替硬碟。

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

影片與音訊存放於 `media_assets`，可搜尋的時間片段存放於 `media_segments`：影片片段保存 `visual_embedding vector(512)`，新版匯入不永久保存 `keyframe_path`；這個 nullable 欄位只保留給舊資料或未來按需產生的 storyboard cache。有音軌的影片及純音訊片段使用 `audio_embedding vector(512)`。逐字稿保留在 asset 與片段層，方便全文搜尋和播放器時間軸顯示。

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

常見相機 RAW 格式會在 Pillow 無法解碼時改由 LibRaw／`rawpy` 產生預覽，包括 DNG、ARW、CR2、CR3、NEF 與 RAF。原始 RAW 檔只會被複製，不會重新編碼。一般照片只解碼一次，並共用同一份已套用 EXIF orientation 的影像來產生 OpenCLIP 向量、1600px 最長邊 AVIF 縮圖與代表色；不會放大小圖。

## 本機影片與音訊辨識

影片與音訊沿用同一個 `embedding-serve` process，且 batch worker 仍一次只處理一個檔案：

- Video：FFmpeg 每 10 秒取一個暫存 keyframe（上限 300），OpenCLIP 為每個 frame 產生 512 維視覺向量。Keyframe 預設最長邊 960px、保留比例且不放大小影片；FFmpeg 通常會在一次 process 中取出同一支影片的所有樣本，只有不足取樣間隔的尾段缺 frame 時才補抓。向量與 Tags 寫入 DB 後，暫存 keyframes 立即刪除，不在 library 永久累積大量小檔案。
- Video audio / Audio：FFmpeg 正規化為 mono 48 kHz WAV，Whisper `base` 產生逐字稿與 timestamp。
- Audio segments：每 30 秒切一段（上限 600），CLAP 為每段產生 512 維聲音向量與聲音類型 Tags。
- Photo 與 Video thumbnail 使用 AVIF。Video thumbnail 是單一 960×540 contact sheet，最多組合兩個代表 frame；不保存兩個獨立檔案。Audio 不產生 thumbnail，Web UI 直接以 CSS 音訊圖示呈現，因此每個 audio 少一次 waveform FFmpeg 處理及一個衍生檔。影片播放仍讀取原始檔，系統不會改變原影片的 container、codec、解析度或長寬比。

需求是 Python 3.12、FFmpeg／ffprobe；模型會在第一次使用時下載，CLAP 權重較大，請預留約 3 GB 的模型快取空間。完成第一次下載後可用完全離線模式啟動：

```bash
make embedding-install
make embedding-serve          # 第一次需連網下載模型
make embedding-serve-offline  # 權重已快取後，不做網路檢查
```

AVIF 預設使用 quality 42、speed 6 的容量優先設定；可用 `DERIVATIVE_IMAGE_QUALITY`（1–100）及 `DERIVATIVE_IMAGE_SPEED`（0–10，越高越快但通常檔案較大）調整。RAW 縮圖另外預設為最長邊 960px、quality 36，優先使用相機內嵌 preview；可用 `RAW_THUMBNAIL_MAX_EDGE` 與 `RAW_THUMBNAIL_QUALITY` 調整。若初次匯入速度比容量重要，可把 speed 提高到 8。另可調整 `PHOTO_THUMBNAIL_MAX_EDGE`、`WHISPER_MODEL`、`WHISPER_LANGUAGE`、`VIDEO_SAMPLE_SECONDS`、`VIDEO_KEYFRAME_MAX_EDGE`、`MAX_VIDEO_SEGMENTS`、`AUDIO_SEGMENT_SECONDS` 與 `MAX_AUDIO_SEGMENTS`。長影片不會卡住 HTTP request：瀏覽器只建立 batch job，真正的 Python 分析在 background worker 執行，進度由 PostgreSQL 保存並透過 SSE 顯示。

照片／影音 inspect 與 import 回應會包含 `analysisTimingsMs`，Python 的 `/v1/analyze` 與 `/v1/analyze-media` 則回傳 `timingsMs`，可分辨 decode、縮圖、keyframe、模型 inference、標籤與轉錄耗時。請先用真實、混合相機來源的 100–1000 個檔案量測，再決定是否更換縮圖 backend：

```bash
make embedding-benchmark \
  BENCHMARK_SOURCE="/Volumes/PHOTO_DISK/sample" \
  BENCHMARK_OUTPUT="$HOME/Pictures/ApoFocus Library/benchmark" \
  BENCHMARK_ARGS="--limit-per-type 100 --json /tmp/apofocus-benchmark.json"
```

`BENCHMARK_SOURCE` 必須位於 `PHOTO_ROOTS`，`BENCHMARK_OUTPUT` 必須位於 `THUMBNAIL_ROOTS`。報告列出每種媒體的 mean／p50／p95 與各階段耗時；至少量到 20 張照片，且真正可替換的 `thumbnailMs` 佔照片分析總時間 20% 以上，才建議用同一批樣本另行評估 libvips。Shared decode、OpenCLIP、Tags 與代表色時間不會被誤算成縮圖 backend 可改善的範圍。

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

這裡不使用 WebSocket，因為進度是 server 到瀏覽器或 MCP client 的單向狀態變化；取消使用獨立操作即可。PostgreSQL job queue 讓 app 意外退出後能從未完成的 item 恢復，並利用媒體 SHA-256 保證重試安全。

- worker 在掃描及單檔模型分析期間持續更新 heartbeat。process 中斷後，重新啟動的 worker 會在 heartbeat stale 兩分鐘後自動接手，並把中斷時仍為 `running` 的 item 退回 `pending`。
- `wait_batch_job` 是最長 30 秒的 MCP long-poll；狀態或 `processedCount` 改變就立即回傳，逾時則回傳未改變狀態。LLM 可以重複呼叫，不需要維持一條涵蓋整批工作的連線。
- `resume_batch_job` 可將 stale、`failed`、`cancelled` 或 `completed_with_errors` 工作重新排入 queue。已成功項目保持不變，只重試 `running`／`failed`／尚未完成的項目；仍有新 heartbeat 的活躍工作會拒絕 resume，避免重複處理。

### Worker 執行與中斷恢復

Batch worker 不是獨立 binary；它和 Web API 一起由 `cmd/apofocus` 啟動。只有在 `DATABASE_URL` 已設定、`PHOTO_LIBRARY_ROOT` 可用，且 `APOFOCUS_IMPORT_ROOTS` 不為空時，Web process 才會啟用 worker。macOS installer 會讓 `com.apofocus.web` LaunchAgent 在 process 異常結束後重新啟動，因此通常不需要使用者或 MCP 手動介入。

Worker 的行為：

- 每秒檢查一次 PostgreSQL queue，一次 claim 一個 job，且一次只處理一個檔案；目前不會並行執行照片、影片或音訊模型。
- 掃描期間會定期保存已找到的 `batch_items` 並更新 heartbeat；單檔 metadata、copy、縮圖、向量或逐字稿分析期間，每 30 秒更新 heartbeat。
- 每個成功 item 立即寫回 PostgreSQL。若 process 中斷，已成功項目不重做；當 heartbeat 超過兩分鐘未更新，任何重新啟動的 worker 都可自動 claim 該 job，並將中斷時的 `running` item 改回 `pending`。
- 取消是 cooperative：worker 完成目前不可分割的單檔步驟、再次檢查 `cancel_requested` 後停止。
- 若 job 已正式進入 `failed`、`cancelled` 或 `completed_with_errors`，worker 不會無限自動重試；可由 UI 或 MCP 的 `resume_batch_job` 明確重新排入 queue。

MCP server 與 worker 是分開的 process。`apofocus-mcp` 可以建立、列出、監控、取消及 resume batch job，但不會自行處理 queue；至少需要一個 `cmd/apofocus` Web process 正在執行。若 Web/worker 沒有重新啟動，job 仍會安全保留在 PostgreSQL，直到 worker 再次上線。

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

設定 `DATABASE_URL` 與 `PHOTO_LIBRARY_ROOT` 後，Web app 與 MCP server 會把這個路徑登記在 `storage_roots`，啟動時確認 DB 已知的原檔、縮圖與舊版可能保存的 keyframe，接著遞迴監聽 library 內的 filesystem events。

- 檔案在同一個 filesystem 內改名或搬到 library 的其他資料夾時，device + inode 不變；watcher 會用 `file_id` 找到原資料並更新 `path`、`relative_path` 與瀏覽器 URL。
- 刪除或移出 library 時，不刪 DB row，而是標成 `missing`；外接硬碟拔除時整個 root 標成 `volume_offline`。若 app 持續執行，重新掛載後會自動恢復監聽並核對已知路徑。
- 新的匯入在寫入 DB 時就保存原檔與 AVIF 縮圖的 file ID，不需要等下一次 scan；視覺取樣幀只在分析期間暫存，不會進入 managed library。
- 啟動時只 `stat` 資料庫已知的路徑，不會掃整個 Volume 計算 SHA-256。只有 filesystem event 指向一個新搬入的資料夾時，才會走訪該子樹以接上其中已知 file ID。

SHA-256 仍用於內容去重；它不能在沒有 index 或 filesystem event 的情況下告訴系統「檔案搬到哪裡」。如果程式關閉期間把檔案搬出 managed library、跨 filesystem 複製後刪除，inode 也會改變，系統只能先標成 `missing`。這種情況之後可再加一個由使用者指定範圍的 relink job，以 size/mtime 縮小候選後才計算 SHA-256，而不是無條件掃所有磁碟。

## MCP：讓 LLM 操作 ApoFocus

MCP server 使用官方 Go SDK 與 stdio transport；啟用備份後完整模式共提供 28 個 tools：

| 類別 | Tools | 行為 |
|---|---|---|
| 照片匯入 | `get_photo_import_policy`、`inspect_photo`、`import_photo` | allowlist、EXIF／GeoTag 預覽、folder、OpenCLIP Tags／向量、縮圖及 transaction 寫入 |
| 照片搜尋 | `search_photos`、`get_photo`、`find_similar_photos` | 年份、專題、Tags、相機、鏡頭、ISO、GeoTag、全文搜尋與 pgvector 相似搜尋 |
| 影片／音訊 | `inspect_media`、`import_media`、`search_media`、`get_media`、`find_similar_media` | metadata、逐字稿、暫存視覺取樣幀、OpenCLIP／CLAP vectors、Tags、搜尋與相似度 |
| 虛擬資料夾 | `browse_folders`、`create_collection`、`add_photos_to_collection`、`get_collection_photos` | facets、Finder 式 browsing、manual／smart collections；`browse_folders.locale` 支援 `zh-TW`、`en`、`de` |
| Batch | `create_batch_job`、`list_batch_jobs`、`get_batch_job`、`wait_batch_job`、`list_batch_items`、`cancel_batch_job`、`resume_batch_job` | 建立與找回持久化工作、短輪詢監控、逐檔錯誤、取消與安全恢復；狀態 label 支援三種語言 |
| 維運 | `get_system_health`、`diagnose_batch_job`、`repair_managed_service` | 檢查 DB、Web／Worker、模型服務、heartbeat、路徑與磁碟空間；診斷後只允許重啟固定 LaunchAgents |
| 備份 | `get_backup_health`、`run_backup`、`verify_backup` | 檢查 Volume UUID、空間、最新備份、還原驗證與錯誤；只允許非同步觸發兩個固定 backup LaunchAgents |

資料夾規則為：

```text
<PHOTO_LIBRARY_ROOT>/originals/YYYY/<project>/YYYY-MM-DD_<filename>_<sha-prefix>.<ext>
<PHOTO_LIBRARY_ROOT>/thumbnails/YYYY/<project>/YYYY-MM-DD_<filename>_<sha-prefix>.avif
```

MCP host 必須先將聊天中附加的照片、影片或音訊落到 `APOFOCUS_IMPORT_ROOTS` 其中一個本機目錄，再把該絕對路徑傳入 `source_path`。server 會解析 symlink 並重新檢查範圍，不接受任意 filesystem path。來源檔一律保留，匯入採 copy。

啟動順序：

```bash
export DATABASE_URL='postgres://apofocus:apofocus@localhost:5432/apofocus?sslmode=disable'
export APOFOCUS_IMPORT_ROOTS='/Users/me/Downloads/ApoFocus-Inbox'
export PHOTO_LIBRARY_ROOT='/Volumes/PhotoArchive/ApoFocus'
export PHOTO_ROOTS="$APOFOCUS_IMPORT_ROOTS"
export THUMBNAIL_ROOTS="$PHOTO_LIBRARY_ROOT"
export EMBEDDING_SERVICE_URL='http://127.0.0.1:8090'
export APOFOCUS_APP_URL='http://127.0.0.1:8080'
export APOFOCUS_BACKUP_ROOT='/Volumes/ApoFocusBackup/PostgreSQL'
export APOFOCUS_BACKUP_STATUS="$HOME/Library/Application Support/ApoFocus/backup-status.json"
export APOFOCUS_BACKUP_VOLUME_UUID='<diskutil reported UUID>'

make embedding-serve   # terminal 1
make run               # terminal 2，包含 Web API 與 Batch Worker
make build-mcp         # 產生 bin/apofocus-mcp，交由 LLM client 啟動
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
        "EMBEDDING_SERVICE_URL": "http://127.0.0.1:8090",
        "APOFOCUS_APP_URL": "http://127.0.0.1:8080",
        "APOFOCUS_BACKUP_ROOT": "/Volumes/ApoFocusBackup/PostgreSQL",
        "APOFOCUS_BACKUP_STATUS": "/Users/me/Library/Application Support/ApoFocus/backup-status.json",
        "APOFOCUS_BACKUP_VOLUME_UUID": "<volume UUID>"
      }
    }
  }
}
```

建議的單檔 LLM 操作順序：

1. 使用者附加照片、影片或音訊，MCP host 將檔案放入 allowlisted inbox。
2. LLM 呼叫 `inspect_photo` 或 `inspect_media`，把 metadata、建議 folder 與 Tags 呈現給使用者。
3. 使用者確認或修改標題、專題、地點與 Tags。
4. LLM 呼叫 `import_photo` 或 `import_media` 並傳入 `confirmed: true`。
5. 回傳 DB ID、實際原檔／縮圖 path、最終 Tags，以及向量或片段摘要。

自動 Tags 完全在本機執行：OpenCLIP 會從可在 [services/embedding/app.py](services/embedding/app.py) 調整的攝影詞彙中挑出最多四個高於信心門檻的標籤；數量與門檻可透過 `AUTO_TAG_LIMIT`、`AUTO_TAG_MIN_SCORE` 調整。LLM 提供的 Tags 會與自動 Tags 合併、去重，而非直接覆蓋。

### Agent 維運流程

使用者不需要讓 Agent 長時間監看 Batch。從遠端回到本地 LLM client 並詢問進度時，Agent 應依序：

1. 呼叫 `get_system_health`，檢查 PostgreSQL、Web／Worker、embedding service、Batch heartbeat、library／import roots 與磁碟空間。
2. 以 `list_batch_jobs` 找回近期工作，再對目標呼叫 `diagnose_batch_job`。診斷會區分正常長時間分析、stale heartbeat、服務停止、來源硬碟離線、library 空間不足和可重試的終止工作。
3. 若診斷建議修復，才呼叫 `repair_managed_service` 並傳入 `confirmed: true`。`service` 只接受 `postgres`、`web` 或 `embedding`，底層只執行對應的 `com.apofocus.*` LaunchAgent restart，不接受 command、argument 或任意 shell input。
4. 再次呼叫 `get_system_health`。服務恢復後，stale 的 active job 會由 Worker 自動接手；只有 `failed`、`cancelled` 或 `completed_with_errors` 才需要呼叫 `resume_batch_job`。
5. 若使用者詢問備份，呼叫 `get_backup_health`。只有在 Volume UUID 與可用空間正常時才呼叫 `run_backup` 或 `verify_backup`，接著用 `get_backup_health` 追蹤 `runningOperation`、`lastBackupAt`、`lastVerifiedAt` 與 `lastError`。

若 PostgreSQL 離線，MCP 會以降級維運模式啟動，保留健康檢查、固定服務修復，以及已設定的備份健康／觸發 tools；若 managed library 離線，則保留 catalog、Batch 查詢與維運 tools，但不載入會寫入 library 的單檔匯入工具。修復 PostgreSQL 或重新掛載 library 後，重新連線 MCP client 即會載入完整 toolset。

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
python3 -m py_compile services/embedding/app.py services/embedding/worker.py services/embedding/benchmark.py
PYTHONPATH=services/embedding python3 -m unittest services.embedding.test_benchmark
```
