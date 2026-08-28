(function (root) {
  "use strict";

  const STORAGE_KEY = "apofocus.locale";
  const DEFAULT_LOCALE = "en";
  const supportedLocales = ["zh-TW", "en", "de"];

  const translations = {
    "zh-TW": {
      "meta.title": "ApoFocus — 攝影媒體資料庫",
      "meta.description": "ApoFocus — 為職業攝影師設計的智慧媒體資料庫",
      "language.label": "語言",
      "nav.filters": "媒體篩選", "nav.home": "ApoFocus 首頁", "nav.closeFilters": "關閉篩選", "nav.primary": "主要功能",
      "nav.folders": "資料夾瀏覽", "nav.featured": "精選", "nav.folderTitle.photos": "以年份、專題、Tags 與器材瀏覽照片",
      "nav.folderTitle.videos": "以年份、專題、Tags 與 Codec 瀏覽影片", "nav.folderTitle.audios": "以年份、專題、Tags 與 Codec 瀏覽音訊",
      "filters.heading": "篩選條件", "common.clear": "清除", "common.close": "關閉", "common.remove": "移除 {label}",
      "filters.project": "專題", "filters.tags": "標籤", "filters.camera": "相機", "filters.allCameras": "所有相機",
      "filters.lens": "鏡頭", "filters.allLenses": "所有鏡頭", "filters.isoRange": "ISO 範圍", "filters.minimum": "最低",
      "filters.maximum": "最高", "filters.geoOnly": "只顯示有 GeoTag", "filters.codec": "Codec", "filters.allCodecs": "所有 Codec",
      "filters.duration": "長度", "filters.allDurations": "所有長度", "filters.duration.short": "5 分鐘內", "filters.duration.medium": "5–30 分鐘",
      "filters.duration.long": "30 分鐘以上", "filters.transcriptOnly": "只顯示有逐字稿", "filters.hasGeo": "有 GeoTag",
      "filters.hasTranscript": "有逐字稿", "filters.search": "搜尋：{query}", "filters.noYears": "尚無年份資料",
      "filters.noProjects": "尚無專題資料", "filters.noTags": "尚無 Tags", "filters.unavailable": "篩選資料暫時無法使用",
      "storage.capacity": "資料庫容量", "topbar.openFilters": "開啟篩選", "topbar.notifications": "通知", "topbar.profile": "攝影資料庫",
      "media.tablist": "媒體類型", "media.photos.tab": "照片", "media.videos.tab": "影片", "media.audios.tab": "音訊",
      "media.photos.label": "照片", "media.photos.all": "全部照片", "media.photos.year": "拍照年份",
      "media.photos.searchPlaceholder": "搜尋照片、專題、地點、器材…", "media.photos.searchAria": "搜尋照片",
      "media.photos.results": "找到 {count} 張照片", "media.photos.count": "{count} 張照片", "media.photos.view": "查看 {title}",
      "media.photos.emptyFiltered": "沒有符合條件的照片", "media.photos.emptyInitial": "尚未匯入照片",
      "media.photos.emptyInitialCopy": "從本機資料夾或外接硬碟批次匯入後，照片會出現在這裡。",
      "media.videos.label": "影片", "media.videos.all": "全部影片", "media.videos.year": "拍攝年份",
      "media.videos.searchPlaceholder": "搜尋影片場景、逐字稿、專題與 Tags…", "media.videos.searchAria": "搜尋影片",
      "media.videos.results": "找到 {count} 支影片", "media.videos.count": "{count} 支影片", "media.videos.view": "查看 {title}",
      "media.videos.emptyFiltered": "沒有符合條件的影片", "media.videos.emptyInitial": "尚未匯入影片",
      "media.videos.emptyInitialCopy": "從本機資料夾或外接硬碟批次匯入後，影片會依場景、逐字稿與聲音建立索引。",
      "media.audios.label": "音訊", "media.audios.all": "全部音訊", "media.audios.year": "錄製年份",
      "media.audios.searchPlaceholder": "搜尋音訊逐字稿、聲音內容、專題與 Tags…", "media.audios.searchAria": "搜尋音訊",
      "media.audios.results": "找到 {count} 段音訊", "media.audios.count": "{count} 段音訊", "media.audios.view": "查看 {title}",
      "media.audios.emptyFiltered": "沒有符合條件的音訊", "media.audios.emptyInitial": "尚未匯入音訊",
      "media.audios.emptyInitialCopy": "從本機資料夾或外接硬碟批次匯入後，音訊會依逐字稿與聲音向量建立索引。",
      "media.loading": "正在讀取資料庫…", "media.loadError": "{media}資料庫暫時無法使用", "media.loadErrorCopy": "請確認伺服器或 PostgreSQL 連線後再試一次。",
      "media.emptyFilteredCopy": "試著放寬篩選條件，或換一個搜尋關鍵字。", "media.clearAll": "清除所有篩選",
      "media.batchImport": "批次匯入", "media.sort.photo": "拍攝日期（新到舊）", "media.sort.recording": "錄製日期（新到舊）",
      "media.viewMode": "檢視方式", "media.gridView": "網格檢視", "media.listView": "列表檢視",
      "detail.previous": "上一張", "detail.next": "下一張", "detail.photoInfo": "拍攝資訊", "detail.location": "地點",
      "detail.tags": "標籤", "detail.file": "檔案", "detail.mediaInfo": "媒體資訊", "detail.transcript": "逐字稿", "detail.segments": "索引片段",
      "field.camera": "相機", "field.lens": "鏡頭", "field.aperture": "光圈", "field.shutter": "快門", "field.focalLength": "焦距",
      "field.format": "格式", "field.dimensions": "尺寸", "field.fileSize": "檔案大小", "field.year": "年份", "field.duration": "長度",
      "field.sampleRate": "取樣率", "field.channels": "聲道", "field.originalStatus": "原檔狀態", "field.thumbnailStatus": "縮圖狀態", "field.previewStatus": "預覽圖狀態",
      "status.available": "可使用", "status.missing": "找不到檔案", "status.volume_offline": "磁碟離線", "status.unknown": "尚未確認",
      "similar.photoAction": "尋找視覺相似照片", "similar.photoHint": "使用 pgvector 計算影像距離", "similar.calculating": "正在計算向量距離…",
      "similar.failed": "相似搜尋暫時無法使用", "similar.photoTitle": "視覺相似照片", "similar.description": "以「{title}」為基準，依 {model} cosine similarity 排序",
      "similar.score": "{score}% 相似", "similar.visualVideoAction": "尋找畫面相似影片", "similar.visualHint": "OpenCLIP keyframe 距離",
      "similar.audioContentAction": "尋找聲音相似內容", "similar.audioVideoAction": "尋找聲音相似影片", "similar.audioAudioAction": "尋找聲音相似音訊",
      "similar.audioHint": "CLAP 音訊片段距離", "similar.visualTitle.videos": "畫面相似影片", "similar.audioTitle.videos": "聲音相似影片",
      "similar.audioTitle.audios": "聲音相似音訊", "similar.noComparable": "目前沒有其他可比較的{media}", "similar.noComparableCopy": "再匯入一些內容後即可計算向量距離。",
      "media.audioWaveform": "音訊波形", "media.noTranscript": "沒有偵測到可辨識的語音。", "media.segmentSummary": "視覺片段 {visual} 個 · 聲音片段 {audio} 個",
      "folder.title": "資料夾瀏覽", "folder.sources": "資料來源", "folder.children": "子資料夾", "folder.chooseCategory": "選擇左側分類",
      "folder.choose": "選擇資料夾", "folder.chooseCopy": "依年份、專題、標籤或器材瀏覽，不會移動實體照片。", "folder.loading": "正在讀取資料夾…",
      "folder.loadError": "資料夾讀取失敗", "folder.myFolders": "我的資料夾", "folder.empty": "這裡還沒有資料夾",
      "folder.manualCopy": "手動收藏的照片，不影響檔案實際位置。", "folder.smartCopy": "這是由資料庫條件即時產生的智慧資料夾。",
      "folder.smartCopyCount": "這是由資料庫條件即時產生的智慧資料夾，包含 {count}。", "folder.browse": "瀏覽{media}", "folder.result": "{name} · {count}",
      "batch.title": "批次整理{media}", "batch.copy.photos": "工作會在本機依序執行；關閉視窗不會中止。",
      "batch.copy.media": "本機會依序建立片段、逐字稿、向量與 Tags；關閉視窗不會中止。", "batch.source": "{media}資料夾或 Volume 路徑",
      "batch.project": "共用專題（可留空）", "batch.projectPlaceholder": "例如：島嶼日常", "batch.tags": "共用 Tags，以逗號分隔",
      "batch.tagsPlaceholder": "紀實, 客戶精選", "batch.recursive": "包含所有子資料夾", "batch.auto.photos": "使用 OpenCLIP 自動加 Tags",
      "batch.auto.videos": "使用 OpenCLIP 與 CLAP 自動加 Tags", "batch.auto.audios": "使用 CLAP 自動加 Tags", "batch.create": "建立批次工作",
      "batch.creating": "正在建立工作…", "batch.createError": "無法建立批次工作", "batch.discovered": "已發現", "batch.success": "成功", "batch.failed": "失敗",
      "batch.cancel": "要求停止工作", "batch.cancelling": "正在停止…", "batch.queue": "工作已存入 PostgreSQL queue",
      "batch.status.pending": "等待處理", "batch.status.scanning": "掃描媒體檔案", "batch.status.running": "處理媒體、片段、逐字稿與向量",
      "batch.status.completed": "處理完成", "batch.status.completed_with_errors": "完成，部分檔案失敗", "batch.status.failed": "工作失敗", "batch.status.cancelled": "已停止"
    },
    en: {
      "meta.title": "ApoFocus — Professional Media Library", "meta.description": "ApoFocus — a smart media library for professional photographers",
      "language.label": "Language", "nav.filters": "Media filters", "nav.home": "ApoFocus home", "nav.closeFilters": "Close filters", "nav.primary": "Main navigation",
      "nav.folders": "Browse folders", "nav.featured": "Featured", "nav.folderTitle.photos": "Browse photos by year, project, tags, and equipment",
      "nav.folderTitle.videos": "Browse videos by year, project, tags, and codec", "nav.folderTitle.audios": "Browse audio by year, project, tags, and codec",
      "filters.heading": "Filters", "common.clear": "Clear", "common.close": "Close", "common.remove": "Remove {label}", "filters.project": "Project", "filters.tags": "Tags",
      "filters.camera": "Camera", "filters.allCameras": "All cameras", "filters.lens": "Lens", "filters.allLenses": "All lenses", "filters.isoRange": "ISO range",
      "filters.minimum": "Min", "filters.maximum": "Max", "filters.geoOnly": "Only show items with GeoTags", "filters.codec": "Codec", "filters.allCodecs": "All codecs",
      "filters.duration": "Duration", "filters.allDurations": "All durations", "filters.duration.short": "Under 5 minutes", "filters.duration.medium": "5–30 minutes",
      "filters.duration.long": "Over 30 minutes", "filters.transcriptOnly": "Only show items with transcripts", "filters.hasGeo": "Has GeoTag", "filters.hasTranscript": "Has transcript",
      "filters.search": "Search: {query}", "filters.noYears": "No year data", "filters.noProjects": "No project data", "filters.noTags": "No tags", "filters.unavailable": "Filters are temporarily unavailable",
      "storage.capacity": "Library storage", "topbar.openFilters": "Open filters", "topbar.notifications": "Notifications", "topbar.profile": "Photo library",
      "media.tablist": "Media type", "media.photos.tab": "Photos", "media.videos.tab": "Videos", "media.audios.tab": "Audio",
      "media.photos.label": "photos", "media.photos.all": "All photos", "media.photos.year": "Year taken", "media.photos.searchPlaceholder": "Search photos, projects, places, equipment…",
      "media.photos.searchAria": "Search photos", "media.photos.results": "{count} photos found", "media.photos.count": "{count} photos", "media.photos.view": "View {title}",
      "media.photos.emptyFiltered": "No photos match your filters", "media.photos.emptyInitial": "No photos imported yet", "media.photos.emptyInitialCopy": "Import a local folder or external drive to see photos here.",
      "media.videos.label": "videos", "media.videos.all": "All videos", "media.videos.year": "Year recorded", "media.videos.searchPlaceholder": "Search scenes, transcripts, projects, and tags…",
      "media.videos.searchAria": "Search videos", "media.videos.results": "{count} videos found", "media.videos.count": "{count} videos", "media.videos.view": "View {title}",
      "media.videos.emptyFiltered": "No videos match your filters", "media.videos.emptyInitial": "No videos imported yet", "media.videos.emptyInitialCopy": "Import a local folder or external drive to index scenes, transcripts, and audio.",
      "media.audios.label": "audio files", "media.audios.all": "All audio", "media.audios.year": "Year recorded", "media.audios.searchPlaceholder": "Search transcripts, sounds, projects, and tags…",
      "media.audios.searchAria": "Search audio", "media.audios.results": "{count} audio files found", "media.audios.count": "{count} audio files", "media.audios.view": "View {title}",
      "media.audios.emptyFiltered": "No audio matches your filters", "media.audios.emptyInitial": "No audio imported yet", "media.audios.emptyInitialCopy": "Import a local folder or external drive to index transcripts and audio embeddings.",
      "media.loading": "Loading library…", "media.loadError": "{media} library is temporarily unavailable", "media.loadErrorCopy": "Check the server or PostgreSQL connection and try again.",
      "media.emptyFilteredCopy": "Try broadening your filters or using a different search term.", "media.clearAll": "Clear all filters", "media.batchImport": "Batch import",
      "media.sort.photo": "Date taken (newest first)", "media.sort.recording": "Date recorded (newest first)", "media.viewMode": "View mode", "media.gridView": "Grid view", "media.listView": "List view",
      "detail.previous": "Previous", "detail.next": "Next", "detail.photoInfo": "Capture details", "detail.location": "Location", "detail.tags": "Tags", "detail.file": "File",
      "detail.mediaInfo": "Media details", "detail.transcript": "Transcript", "detail.segments": "Indexed segments", "field.camera": "Camera", "field.lens": "Lens", "field.aperture": "Aperture",
      "field.shutter": "Shutter", "field.focalLength": "Focal length", "field.format": "Format", "field.dimensions": "Dimensions", "field.fileSize": "File size", "field.year": "Year",
      "field.duration": "Duration", "field.sampleRate": "Sample rate", "field.channels": "Channels", "field.originalStatus": "Original status", "field.thumbnailStatus": "Thumbnail status", "field.previewStatus": "Preview status",
      "status.available": "Available", "status.missing": "File missing", "status.volume_offline": "Drive offline", "status.unknown": "Not checked",
      "similar.photoAction": "Find visually similar photos", "similar.photoHint": "Calculate image distance with pgvector", "similar.calculating": "Calculating vector distance…", "similar.failed": "Similarity search is temporarily unavailable",
      "similar.photoTitle": "Visually similar photos", "similar.description": "Compared with “{title}”, sorted by {model} cosine similarity", "similar.score": "{score}% similar",
      "similar.visualVideoAction": "Find visually similar videos", "similar.visualHint": "OpenCLIP keyframe distance", "similar.audioContentAction": "Find similar sounds",
      "similar.audioVideoAction": "Find videos with similar audio", "similar.audioAudioAction": "Find similar audio", "similar.audioHint": "CLAP audio-segment distance",
      "similar.visualTitle.videos": "Visually similar videos", "similar.audioTitle.videos": "Videos with similar audio", "similar.audioTitle.audios": "Similar audio",
      "similar.noComparable": "No other {media} to compare yet", "similar.noComparableCopy": "Import more content to calculate vector distance.",
      "media.audioWaveform": "Audio waveform", "media.noTranscript": "No recognizable speech was detected.", "media.segmentSummary": "{visual} visual segments · {audio} audio segments",
      "folder.title": "Browse folders", "folder.sources": "Sources", "folder.children": "Subfolders", "folder.chooseCategory": "Choose a category on the left", "folder.choose": "Choose a folder",
      "folder.chooseCopy": "Browse by year, project, tags, or equipment without moving physical files.", "folder.loading": "Loading folders…", "folder.loadError": "Could not load folders",
      "folder.myFolders": "My folders", "folder.empty": "No folders here yet", "folder.manualCopy": "A manual photo collection that does not change file locations.",
      "folder.smartCopy": "A smart folder generated live from database filters.", "folder.smartCopyCount": "A smart folder generated live from database filters, containing {count}.",
      "folder.browse": "Browse {media}", "folder.result": "{name} · {count}", "batch.title": "Organize {media} in a batch", "batch.copy.photos": "Runs sequentially on this Mac; closing the dialog will not stop it.",
      "batch.copy.media": "Creates segments, transcripts, vectors, and tags sequentially on this Mac; closing the dialog will not stop it.", "batch.source": "{media} folder or volume path",
      "batch.project": "Shared project (optional)", "batch.projectPlaceholder": "e.g. Island Life", "batch.tags": "Shared tags, comma separated", "batch.tagsPlaceholder": "documentary, client picks",
      "batch.recursive": "Include all subfolders", "batch.auto.photos": "Add tags automatically with OpenCLIP", "batch.auto.videos": "Add tags automatically with OpenCLIP and CLAP",
      "batch.auto.audios": "Add tags automatically with CLAP", "batch.create": "Create batch job", "batch.creating": "Creating job…", "batch.createError": "Could not create batch job",
      "batch.discovered": "Discovered", "batch.success": "Succeeded", "batch.failed": "Failed", "batch.cancel": "Stop job", "batch.cancelling": "Stopping…", "batch.queue": "Job saved to the PostgreSQL queue",
      "batch.status.pending": "Waiting", "batch.status.scanning": "Scanning media files", "batch.status.running": "Processing media, segments, transcripts, and vectors",
      "batch.status.completed": "Completed", "batch.status.completed_with_errors": "Completed with some failures", "batch.status.failed": "Job failed", "batch.status.cancelled": "Stopped"
    },
    de: {
      "meta.title": "ApoFocus — Professionelles Medienarchiv", "meta.description": "ApoFocus — eine intelligente Medienbibliothek für professionelle Fotografinnen und Fotografen",
      "language.label": "Sprache", "nav.filters": "Medienfilter", "nav.home": "ApoFocus-Startseite", "nav.closeFilters": "Filter schließen", "nav.primary": "Hauptnavigation",
      "nav.folders": "Ordner durchsuchen", "nav.featured": "Auswahl", "nav.folderTitle.photos": "Fotos nach Jahr, Projekt, Tags und Ausrüstung durchsuchen",
      "nav.folderTitle.videos": "Videos nach Jahr, Projekt, Tags und Codec durchsuchen", "nav.folderTitle.audios": "Audio nach Jahr, Projekt, Tags und Codec durchsuchen",
      "filters.heading": "Filter", "common.clear": "Leeren", "common.close": "Schließen", "common.remove": "{label} entfernen", "filters.project": "Projekt", "filters.tags": "Tags",
      "filters.camera": "Kamera", "filters.allCameras": "Alle Kameras", "filters.lens": "Objektiv", "filters.allLenses": "Alle Objektive", "filters.isoRange": "ISO-Bereich",
      "filters.minimum": "Min.", "filters.maximum": "Max.", "filters.geoOnly": "Nur Elemente mit GeoTags", "filters.codec": "Codec", "filters.allCodecs": "Alle Codecs",
      "filters.duration": "Dauer", "filters.allDurations": "Alle Längen", "filters.duration.short": "Unter 5 Minuten", "filters.duration.medium": "5–30 Minuten",
      "filters.duration.long": "Über 30 Minuten", "filters.transcriptOnly": "Nur Elemente mit Transkript", "filters.hasGeo": "Mit GeoTag", "filters.hasTranscript": "Mit Transkript",
      "filters.search": "Suche: {query}", "filters.noYears": "Keine Jahresdaten", "filters.noProjects": "Keine Projektdaten", "filters.noTags": "Keine Tags", "filters.unavailable": "Filter sind vorübergehend nicht verfügbar",
      "storage.capacity": "Speicherbelegung", "topbar.openFilters": "Filter öffnen", "topbar.notifications": "Benachrichtigungen", "topbar.profile": "Fotoarchiv",
      "media.tablist": "Medientyp", "media.photos.tab": "Fotos", "media.videos.tab": "Videos", "media.audios.tab": "Audio",
      "media.photos.label": "Fotos", "media.photos.all": "Alle Fotos", "media.photos.year": "Aufnahmejahr", "media.photos.searchPlaceholder": "Fotos, Projekte, Orte und Ausrüstung durchsuchen…",
      "media.photos.searchAria": "Fotos durchsuchen", "media.photos.results": "{count} Fotos gefunden", "media.photos.count": "{count} Fotos", "media.photos.view": "{title} ansehen",
      "media.photos.emptyFiltered": "Keine Fotos entsprechen den Filtern", "media.photos.emptyInitial": "Noch keine Fotos importiert", "media.photos.emptyInitialCopy": "Importiere einen lokalen Ordner oder ein externes Laufwerk, um hier Fotos zu sehen.",
      "media.videos.label": "Videos", "media.videos.all": "Alle Videos", "media.videos.year": "Aufnahmejahr", "media.videos.searchPlaceholder": "Szenen, Transkripte, Projekte und Tags durchsuchen…",
      "media.videos.searchAria": "Videos durchsuchen", "media.videos.results": "{count} Videos gefunden", "media.videos.count": "{count} Videos", "media.videos.view": "{title} ansehen",
      "media.videos.emptyFiltered": "Keine Videos entsprechen den Filtern", "media.videos.emptyInitial": "Noch keine Videos importiert", "media.videos.emptyInitialCopy": "Importiere einen lokalen Ordner oder ein externes Laufwerk, um Szenen, Transkripte und Audio zu indexieren.",
      "media.audios.label": "Audiodateien", "media.audios.all": "Alle Audiodateien", "media.audios.year": "Aufnahmejahr", "media.audios.searchPlaceholder": "Transkripte, Klänge, Projekte und Tags durchsuchen…",
      "media.audios.searchAria": "Audio durchsuchen", "media.audios.results": "{count} Audiodateien gefunden", "media.audios.count": "{count} Audiodateien", "media.audios.view": "{title} ansehen",
      "media.audios.emptyFiltered": "Keine Audiodateien entsprechen den Filtern", "media.audios.emptyInitial": "Noch keine Audiodateien importiert", "media.audios.emptyInitialCopy": "Importiere einen lokalen Ordner oder ein externes Laufwerk, um Transkripte und Audiovektoren zu indexieren.",
      "media.loading": "Archiv wird geladen…", "media.loadError": "Das Archiv für {media} ist vorübergehend nicht verfügbar", "media.loadErrorCopy": "Server- oder PostgreSQL-Verbindung prüfen und erneut versuchen.",
      "media.emptyFilteredCopy": "Erweitere die Filter oder verwende einen anderen Suchbegriff.", "media.clearAll": "Alle Filter leeren", "media.batchImport": "Stapelimport",
      "media.sort.photo": "Aufnahmedatum (neueste zuerst)", "media.sort.recording": "Aufzeichnungsdatum (neueste zuerst)", "media.viewMode": "Ansicht", "media.gridView": "Rasteransicht", "media.listView": "Listenansicht",
      "detail.previous": "Zurück", "detail.next": "Weiter", "detail.photoInfo": "Aufnahmedaten", "detail.location": "Ort", "detail.tags": "Tags", "detail.file": "Datei",
      "detail.mediaInfo": "Mediendaten", "detail.transcript": "Transkript", "detail.segments": "Indexierte Segmente", "field.camera": "Kamera", "field.lens": "Objektiv", "field.aperture": "Blende",
      "field.shutter": "Verschlusszeit", "field.focalLength": "Brennweite", "field.format": "Format", "field.dimensions": "Abmessungen", "field.fileSize": "Dateigröße", "field.year": "Jahr",
      "field.duration": "Dauer", "field.sampleRate": "Abtastrate", "field.channels": "Kanäle", "field.originalStatus": "Status des Originals", "field.thumbnailStatus": "Thumbnail-Status", "field.previewStatus": "Vorschaustatus",
      "status.available": "Verfügbar", "status.missing": "Datei fehlt", "status.volume_offline": "Laufwerk offline", "status.unknown": "Nicht geprüft",
      "similar.photoAction": "Visuell ähnliche Fotos finden", "similar.photoHint": "Bildabstand mit pgvector berechnen", "similar.calculating": "Vektorabstand wird berechnet…", "similar.failed": "Ähnlichkeitssuche ist vorübergehend nicht verfügbar",
      "similar.photoTitle": "Visuell ähnliche Fotos", "similar.description": "Verglichen mit „{title}“, sortiert nach {model}-Kosinusähnlichkeit", "similar.score": "{score} % ähnlich",
      "similar.visualVideoAction": "Visuell ähnliche Videos finden", "similar.visualHint": "OpenCLIP-Keyframe-Abstand", "similar.audioContentAction": "Ähnliche Klänge finden",
      "similar.audioVideoAction": "Videos mit ähnlichem Ton finden", "similar.audioAudioAction": "Ähnliche Audiodateien finden", "similar.audioHint": "CLAP-Audiosegment-Abstand",
      "similar.visualTitle.videos": "Visuell ähnliche Videos", "similar.audioTitle.videos": "Videos mit ähnlichem Ton", "similar.audioTitle.audios": "Ähnliche Audiodateien",
      "similar.noComparable": "Noch keine anderen {media} zum Vergleichen", "similar.noComparableCopy": "Importiere weitere Inhalte, um Vektorabstände zu berechnen.",
      "media.audioWaveform": "Audiowellenform", "media.noTranscript": "Keine erkennbare Sprache gefunden.", "media.segmentSummary": "{visual} visuelle Segmente · {audio} Audiosegmente",
      "folder.title": "Ordner durchsuchen", "folder.sources": "Quellen", "folder.children": "Unterordner", "folder.chooseCategory": "Links eine Kategorie auswählen", "folder.choose": "Ordner auswählen",
      "folder.chooseCopy": "Nach Jahr, Projekt, Tags oder Ausrüstung suchen, ohne physische Dateien zu verschieben.", "folder.loading": "Ordner werden geladen…", "folder.loadError": "Ordner konnten nicht geladen werden",
      "folder.myFolders": "Meine Ordner", "folder.empty": "Hier gibt es noch keine Ordner", "folder.manualCopy": "Eine manuelle Fotosammlung, die Dateipfade nicht verändert.",
      "folder.smartCopy": "Ein intelligenter Ordner, der live aus Datenbankfiltern erzeugt wird.", "folder.smartCopyCount": "Ein intelligenter Ordner aus Datenbankfiltern mit {count}.",
      "folder.browse": "{media} durchsuchen", "folder.result": "{name} · {count}", "batch.title": "{media} stapelweise organisieren", "batch.copy.photos": "Wird auf diesem Mac nacheinander ausgeführt; das Schließen des Dialogs stoppt den Vorgang nicht.",
      "batch.copy.media": "Segmente, Transkripte, Vektoren und Tags werden auf diesem Mac nacheinander erstellt; das Schließen des Dialogs stoppt den Vorgang nicht.", "batch.source": "Ordner- oder Volume-Pfad für {media}",
      "batch.project": "Gemeinsames Projekt (optional)", "batch.projectPlaceholder": "z. B. Inselleben", "batch.tags": "Gemeinsame Tags, durch Kommas getrennt", "batch.tagsPlaceholder": "Dokumentation, Kundenauswahl",
      "batch.recursive": "Alle Unterordner einbeziehen", "batch.auto.photos": "Tags automatisch mit OpenCLIP hinzufügen", "batch.auto.videos": "Tags automatisch mit OpenCLIP und CLAP hinzufügen",
      "batch.auto.audios": "Tags automatisch mit CLAP hinzufügen", "batch.create": "Stapelauftrag erstellen", "batch.creating": "Auftrag wird erstellt…", "batch.createError": "Stapelauftrag konnte nicht erstellt werden",
      "batch.discovered": "Gefunden", "batch.success": "Erfolgreich", "batch.failed": "Fehlgeschlagen", "batch.cancel": "Auftrag stoppen", "batch.cancelling": "Wird gestoppt…", "batch.queue": "Auftrag wurde in der PostgreSQL-Warteschlange gespeichert",
      "batch.status.pending": "Wartet", "batch.status.scanning": "Mediendateien werden gescannt", "batch.status.running": "Medien, Segmente, Transkripte und Vektoren werden verarbeitet",
      "batch.status.completed": "Abgeschlossen", "batch.status.completed_with_errors": "Mit einigen Fehlern abgeschlossen", "batch.status.failed": "Auftrag fehlgeschlagen", "batch.status.cancelled": "Gestoppt"
    }
  };

  function normalizeLocale(value) {
    const locale = String(value || "").replace("_", "-").toLowerCase();
    if (locale === "zh" || locale.startsWith("zh-")) return "zh-TW";
    if (locale === "de" || locale.startsWith("de-")) return "de";
    if (locale === "en" || locale.startsWith("en-")) return "en";
    return null;
  }

  function detectLocale() {
    try {
      const stored = root.document ? normalizeLocale(root.localStorage?.getItem(STORAGE_KEY)) : null;
      if (stored) return stored;
    } catch (_) {}
    const candidates = root.navigator?.languages || [root.navigator?.language];
    for (const candidate of candidates || []) {
      const locale = normalizeLocale(candidate);
      if (locale) return locale;
    }
    return DEFAULT_LOCALE;
  }

  let currentLocale = detectLocale();

  function t(key, params = {}) {
    const template = translations[currentLocale]?.[key] ?? translations[DEFAULT_LOCALE]?.[key] ?? key;
    return template.replace(/\{([^}]+)\}/g, (_, name) => params[name] ?? `{${name}}`);
  }

  function formatNumber(value, options) {
    return new Intl.NumberFormat(currentLocale, options).format(value);
  }

  function formatDate(value, options = { year: "numeric", month: "long", day: "numeric", hour: "2-digit", minute: "2-digit" }) {
    return new Intl.DateTimeFormat(currentLocale, options).format(new Date(value));
  }

  function applyTranslations(scope) {
    if (!root.document) return;
    const base = scope || root.document;
    base.querySelectorAll?.("[data-i18n]").forEach((element) => { element.textContent = t(element.dataset.i18n); });
    base.querySelectorAll?.("[data-i18n-placeholder]").forEach((element) => { element.placeholder = t(element.dataset.i18nPlaceholder); });
    base.querySelectorAll?.("[data-i18n-aria-label]").forEach((element) => { element.setAttribute("aria-label", t(element.dataset.i18nAriaLabel)); });
    base.querySelectorAll?.("[data-i18n-alt]").forEach((element) => { element.alt = t(element.dataset.i18nAlt); });
    base.querySelectorAll?.("[data-i18n-title]").forEach((element) => { element.title = t(element.dataset.i18nTitle); });
    root.document.documentElement.lang = currentLocale === "zh-TW" ? "zh-Hant" : currentLocale;
    root.document.title = t("meta.title");
    const description = root.document.querySelector('meta[name="description"]');
    if (description) description.content = t("meta.description");
    const select = root.document.querySelector("#locale-select");
    if (select) select.value = currentLocale;
  }

  function setLocale(locale, { notify = true } = {}) {
    const normalized = normalizeLocale(locale);
    if (!normalized) return false;
    const changed = normalized !== currentLocale;
    currentLocale = normalized;
    try { if (root.document) root.localStorage?.setItem(STORAGE_KEY, currentLocale); } catch (_) {}
    applyTranslations();
    if (notify && changed && root.dispatchEvent && root.CustomEvent) {
      root.dispatchEvent(new CustomEvent("apofocus:localechange", { detail: { locale: currentLocale } }));
    }
    return true;
  }

  function init() {
    applyTranslations();
    const select = root.document?.querySelector("#locale-select");
    select?.addEventListener("change", (event) => setLocale(event.target.value));
  }

  const api = { STORAGE_KEY, supportedLocales, translations, normalizeLocale, getLocale: () => currentLocale, t, formatNumber, formatDate, applyTranslations, setLocale };
  root.ApoFocusI18n = api;
  if (typeof module !== "undefined" && module.exports) module.exports = api;
  if (root.document) {
    if (root.document.readyState === "loading") root.document.addEventListener("DOMContentLoaded", init);
    else init();
  }
})(typeof window !== "undefined" ? window : globalThis);
