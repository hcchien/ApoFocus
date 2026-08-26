const mediaConfig = {
  photos: {
    label: "照片", title: "全部照片", eyebrow: "PHOTO ARCHIVE", yearLabel: "拍照年份",
    searchPlaceholder: "搜尋照片、專題、地點、器材…", singular: "photo", unit: "張",
  },
  videos: {
    label: "影片", title: "全部影片", eyebrow: "VIDEO ARCHIVE", yearLabel: "拍攝年份",
    searchPlaceholder: "搜尋影片場景、逐字稿、專題與 Tags…", singular: "video", unit: "支",
    emptyTitle: "尚未匯入影片", emptyCopy: "從本機資料夾或外接硬碟批次匯入後，影片會依場景、逐字稿與聲音建立索引。",
  },
  audios: {
    label: "音訊", title: "全部音訊", eyebrow: "AUDIO ARCHIVE", yearLabel: "錄製年份",
    searchPlaceholder: "搜尋音訊逐字稿、聲音內容、專題與 Tags…", singular: "audio", unit: "段",
    emptyTitle: "尚未匯入音訊", emptyCopy: "從本機資料夾或外接硬碟批次匯入後，音訊會依逐字稿與聲音向量建立索引。",
  },
};

const requestedMedia = new URLSearchParams(window.location.search).get("media");

const state = {
  mediaType: mediaConfig[requestedMedia] ? requestedMedia : "photos",
  facets: null,
  mediaFacets: { videos: null, audios: null },
  photos: [],
  mediaItems: [],
  total: 0,
  selected: null,
  selectedMedia: null,
  folderTree: null,
  selectedFolder: null,
  batchJobID: null,
  batchEvents: null,
  filters: { q: "", year: "", project: "", tags: [], camera: "", lens: "", minISO: "", maxISO: "", location: false, codec: "", duration: "", transcript: false },
};

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
const grid = $("#photo-grid");
const dialog = $("#photo-dialog");
const similarDialog = $("#similar-dialog");
const mediaDialog = $("#media-dialog");

document.addEventListener("DOMContentLoaded", init);

async function init() {
  bindEvents();
  applyMediaUI({ syncURL: false });
  await Promise.all([loadFacets(), loadPhotos()]);
}

function bindEvents() {
  $$("[data-media]").forEach((button) => {
    button.addEventListener("click", () => selectMedia(button.dataset.media));
    button.addEventListener("keydown", onMediaTabKeydown);
  });
  let searchTimer;
  $("#search").addEventListener("input", (event) => {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => { state.filters.q = event.target.value.trim(); loadPhotos(); }, 240);
  });
  document.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
      event.preventDefault(); $("#search").focus();
    }
  });
  $$(".section-toggle").forEach((button) => button.addEventListener("click", () => {
    const expanded = button.getAttribute("aria-expanded") === "true";
    button.setAttribute("aria-expanded", String(!expanded));
    button.nextElementSibling.hidden = expanded;
  }));
  $("#year-filters").addEventListener("click", onFacetClick);
  $("#project-filters").addEventListener("click", onFacetClick);
  $("#tag-filters").addEventListener("click", onTagClick);
  $("#camera-filter").addEventListener("change", (event) => { state.filters.camera = event.target.value; loadPhotos(); });
  $("#lens-filter").addEventListener("change", (event) => { state.filters.lens = event.target.value; loadPhotos(); });
  [$("#min-iso"), $("#max-iso")].forEach((input) => input.addEventListener("change", () => {
    state.filters.minISO = $("#min-iso").value; state.filters.maxISO = $("#max-iso").value; loadPhotos();
  }));
  $("#location-filter").addEventListener("change", (event) => { state.filters.location = event.target.checked; loadPhotos(); });
  $("#codec-filter").addEventListener("change", (event) => { state.filters.codec = event.target.value; loadPhotos(); });
  $("#duration-filter").addEventListener("change", (event) => { state.filters.duration = event.target.value; loadPhotos(); });
  $("#transcript-filter").addEventListener("change", (event) => { state.filters.transcript = event.target.checked; loadPhotos(); });
  $("#clear-filters").addEventListener("click", clearFilters);
  $("#empty-clear").addEventListener("click", clearFilters);
  $("#open-filters").addEventListener("click", openSidebar);
  $("#close-filters").addEventListener("click", closeSidebar);
  $("#sidebar-scrim").addEventListener("click", closeSidebar);
  $("#dialog-close").addEventListener("click", () => dialog.close());
  $("#similar-close").addEventListener("click", () => similarDialog.close());
  $("#media-dialog-close").addEventListener("click", closeMediaDialog);
  dialog.addEventListener("click", (event) => { if (event.target === dialog) dialog.close(); });
  similarDialog.addEventListener("click", (event) => { if (event.target === similarDialog) similarDialog.close(); });
  mediaDialog.addEventListener("click", (event) => { if (event.target === mediaDialog) closeMediaDialog(); });
  $("#similar-button").addEventListener("click", showSimilar);
  $("#media-similar-visual").addEventListener("click", () => showSimilarMedia("visual"));
  $("#media-similar-audio").addEventListener("click", () => showSimilarMedia("audio"));
  $("#detail-prev").addEventListener("click", () => navigateDetail(-1));
  $("#detail-next").addEventListener("click", () => navigateDetail(1));
  $("#open-folders").addEventListener("click", openFolderBrowser);
  $("#folder-close").addEventListener("click", () => $("#folder-dialog").close());
  $("#open-batch").addEventListener("click", () => $("#batch-dialog").showModal());
  $("#batch-close").addEventListener("click", () => $("#batch-dialog").close());
  $("#batch-form").addEventListener("submit", submitBatch);
  $("#batch-cancel").addEventListener("click", cancelBatch);
}

async function loadFacets() {
  const mediaType = state.mediaType;
  try {
    const endpoint = mediaType === "photos" ? "/api/v1/facets" : `/api/v1/${mediaType}/facets`;
    const response = await fetch(endpoint);
    if (!response.ok) throw new Error("無法讀取篩選資料");
    const facets = await response.json();
    if (mediaType === "photos") state.facets = facets;
    else state.mediaFacets[mediaType] = facets;
    if (state.mediaType === mediaType) renderFacetsForCurrentMedia();
  } catch (error) {
    console.error(error);
    $("#year-filters").innerHTML = `<p class="result-summary">篩選資料暫時無法使用</p>`;
  }
}

async function loadPhotos() {
  if (state.mediaType !== "photos") {
    return loadMediaAssets();
  }
  renderSkeletons();
  const params = new URLSearchParams({ limit: "60" });
  const f = state.filters;
  if (f.q) params.set("q", f.q);
  if (f.year) params.set("year", f.year);
  if (f.project) params.set("project", f.project);
  f.tags.forEach((tag) => params.append("tag", tag));
  if (f.camera) params.set("camera", f.camera);
  if (f.lens) params.set("lens", f.lens);
  if (f.minISO) params.set("min_iso", f.minISO);
  if (f.maxISO) params.set("max_iso", f.maxISO);
  if (f.location) params.set("has_location", "true");
  try {
    const response = await fetch(`/api/v1/photos?${params}`);
    if (!response.ok) throw new Error("無法讀取照片");
    const page = await response.json();
    state.photos = page.items || [];
    state.total = page.total || 0;
    renderPhotos();
    renderActiveFilters();
    if (!f.q && !f.year && !f.project && !f.tags.length && !f.camera && !f.lens && !f.minISO && !f.maxISO && !f.location) {
      $("#all-count").textContent = new Intl.NumberFormat("zh-TW").format(state.total);
      $("#photos-tab-count").textContent = new Intl.NumberFormat("zh-TW").format(state.total);
    }
  } catch (error) {
    console.error(error);
    grid.innerHTML = `<div class="empty-state"><h2>照片資料庫暫時無法使用</h2><p>請確認伺服器或 PostgreSQL 連線後再試一次。</p></div>`;
  }
}

async function loadMediaAssets() {
  const mediaType = state.mediaType;
  renderSkeletons(true);
  const params = new URLSearchParams({ limit: "60" });
  const f = state.filters;
  if (f.q) params.set("q", f.q);
  if (f.year) params.set("year", f.year);
  if (f.project) params.set("project", f.project);
  f.tags.forEach((tag) => params.append("tag", tag));
  if (f.codec) params.set("codec", f.codec);
  if (f.duration === "short") params.set("max_duration_ms", "300000");
  if (f.duration === "medium") { params.set("min_duration_ms", "300000"); params.set("max_duration_ms", "1800000"); }
  if (f.duration === "long") params.set("min_duration_ms", "1800000");
  if (f.transcript) params.set("has_transcript", "true");
  try {
    const response = await fetch(`/api/v1/${mediaType}?${params}`);
    if (!response.ok) throw new Error(`無法讀取${mediaConfig[mediaType].label}`);
    const page = await response.json();
    if (state.mediaType !== mediaType) return;
    state.mediaItems = page.items || [];
    state.total = page.total || 0;
    renderMediaAssets();
    renderActiveFilters();
    const count = new Intl.NumberFormat("zh-TW").format(state.total);
    $(`#${mediaType}-tab-count`).textContent = count;
    $("#all-count").textContent = count;
  } catch (error) {
    console.error(error);
    grid.innerHTML = `<div class="empty-state"><h2>${escapeHTML(mediaConfig[mediaType].label)}資料庫暫時無法使用</h2><p>請確認伺服器或 PostgreSQL 連線後再試一次。</p></div>`;
  }
}

function renderFacets() {
  const facets = state.facets;
  $("#year-filters").innerHTML = facets.years.map((item) => facetButton("year", item)).join("");
  $("#project-filters").innerHTML = facets.projects.map((item) => facetButton("project", item)).join("");
  $("#tag-filters").innerHTML = facets.tags.slice(0, 12).map((item) => `<button class="tag-pill" data-tag="${escapeAttr(item.value)}">${escapeHTML(item.value)} <span>${item.count}</span></button>`).join("");
  fillSelect($("#camera-filter"), facets.cameras);
  fillSelect($("#lens-filter"), facets.lenses);
}

function renderFacetsForCurrentMedia() {
  const facets = state.mediaType === "photos" ? state.facets : state.mediaFacets[state.mediaType];
  if (!facets) return;
  $("#year-filters").innerHTML = (facets.years || []).map((item) => facetButton("year", item)).join("") || `<p class="facet-empty">尚無年份資料</p>`;
  $("#project-filters").innerHTML = (facets.projects || []).map((item) => facetButton("project", item)).join("") || `<p class="facet-empty">尚無專題資料</p>`;
  $("#tag-filters").innerHTML = (facets.tags || []).slice(0, 12).map((item) => `<button class="tag-pill" data-tag="${escapeAttr(item.value)}">${escapeHTML(item.value)} <span>${item.count}</span></button>`).join("") || `<p class="facet-empty">尚無 Tags</p>`;
  if (state.mediaType === "photos") {
    resetFacetSelect($("#camera-filter"), "所有相機");
    resetFacetSelect($("#lens-filter"), "所有鏡頭");
    fillSelect($("#camera-filter"), facets.cameras || []);
    fillSelect($("#lens-filter"), facets.lenses || []);
    $("#camera-filter").value = state.filters.camera;
    $("#lens-filter").value = state.filters.lens;
  } else {
    resetFacetSelect($("#codec-filter"), "所有 Codec");
    fillSelect($("#codec-filter"), facets.codecs || []);
    $("#codec-filter").value = state.filters.codec;
  }
  syncFacetUI();
}

function resetFacetSelect(select, label) {
  select.innerHTML = `<option value="">${label}</option>`;
}

function facetButton(kind, item) {
  return `<button class="facet-row" data-kind="${kind}" data-value="${escapeAttr(item.value)}"><span>${escapeHTML(item.value)}</span><span class="count">${item.count}</span></button>`;
}

function fillSelect(select, items) {
  items.forEach((item) => select.insertAdjacentHTML("beforeend", `<option value="${escapeAttr(item.value)}">${escapeHTML(item.value)} (${item.count})</option>`));
}

function onFacetClick(event) {
  const button = event.target.closest("[data-kind]");
  if (!button) return;
  const kind = button.dataset.kind;
  state.filters[kind] = state.filters[kind] === button.dataset.value ? "" : button.dataset.value;
  syncFacetUI(); loadPhotos();
}

function onTagClick(event) {
  const button = event.target.closest("[data-tag]");
  if (!button) return;
  const tag = button.dataset.tag;
  state.filters.tags = state.filters.tags.includes(tag) ? state.filters.tags.filter((value) => value !== tag) : [...state.filters.tags, tag];
  syncFacetUI(); loadPhotos();
}

function renderPhotos() {
  $("#result-summary").textContent = `找到 ${new Intl.NumberFormat("zh-TW").format(state.total)} 張照片`;
  renderPhotoEmptyState();
  $("#empty-state").hidden = state.photos.length > 0;
  grid.hidden = state.photos.length === 0;
  grid.innerHTML = state.photos.map((photo) => `
    <button class="photo-card ${photo.aspectRatio === "portrait" ? "portrait" : ""}" data-photo-id="${escapeAttr(photo.id)}" style="background:${escapeAttr(photo.dominantColor)}" aria-label="查看 ${escapeAttr(photo.title)}">
      <img src="${escapeAttr(photo.thumbnailUrl)}" alt="" loading="lazy" decoding="async">
      <span class="card-badge">${escapeHTML(photo.fileType)}</span>
      <span class="card-copy"><h3>${escapeHTML(photo.title)}</h3><p>${escapeHTML(photo.project)} · ${photo.year}</p></span>
    </button>`).join("");
  $$(".photo-card", grid).forEach((card) => card.addEventListener("click", () => openDetail(card.dataset.photoId)));
}

function renderPhotoEmptyState() {
  $("#empty-symbol").textContent = "0";
  $("#empty-title").textContent = "沒有符合條件的照片";
  $("#empty-copy").textContent = "試著放寬篩選條件，或換一個搜尋關鍵字。";
  $("#empty-clear").hidden = false;
}

function renderMediaAssets() {
  const config = mediaConfig[state.mediaType];
  $("#result-summary").textContent = `找到 ${new Intl.NumberFormat("zh-TW").format(state.total)} ${config.unit}${config.label}`;
  const filtered = hasActiveFilters();
  $("#empty-symbol").textContent = state.mediaType === "videos" ? "▶" : "♪";
  $("#empty-title").textContent = filtered ? `沒有符合條件的${config.label}` : config.emptyTitle;
  $("#empty-copy").textContent = filtered ? "試著放寬篩選條件，或換一個搜尋關鍵字。" : config.emptyCopy;
  $("#empty-clear").hidden = !filtered;
  $("#empty-state").hidden = state.mediaItems.length > 0;
  grid.hidden = state.mediaItems.length === 0;
  grid.innerHTML = state.mediaItems.map((asset) => `
    <button class="photo-card media-card" data-media-id="${escapeAttr(asset.id)}" aria-label="查看 ${escapeAttr(asset.title)}">
      <img src="${escapeAttr(asset.thumbnailUrl)}" alt="" loading="lazy" decoding="async">
      <span class="media-kind">${state.mediaType === "videos" ? "▶" : "♪"}</span>
      <span class="card-badge">${escapeHTML(formatDuration(asset.durationMs))}</span>
      <span class="card-copy"><h3>${escapeHTML(asset.title)}</h3><p>${escapeHTML(asset.project)} · ${asset.year} · ${escapeHTML(asset.codec)}</p></span>
    </button>`).join("");
  $$('[data-media-id]', grid).forEach((card) => card.addEventListener("click", () => openMediaDetail(card.dataset.mediaId)));
}

function renderSkeletons(media = false) {
  $("#empty-state").hidden = true;
  grid.hidden = false;
  grid.innerHTML = Array.from({ length: media ? 8 : 12 }, (_, index) => `<div class="photo-skeleton ${!media && index % 4 === 1 ? "tall" : ""}"></div>`).join("");
}

function selectMedia(mediaType) {
  if (!mediaConfig[mediaType] || mediaType === state.mediaType) return;
  state.mediaType = mediaType;
  if (mediaType === "photos") {
    state.filters.codec = ""; state.filters.duration = ""; state.filters.transcript = false;
  } else {
    state.filters.camera = ""; state.filters.lens = ""; state.filters.minISO = ""; state.filters.maxISO = ""; state.filters.location = false;
  }
  applyMediaUI({ syncURL: true });
  Promise.all([loadFacets(), loadPhotos()]);
}

function applyMediaUI({ syncURL }) {
  const config = mediaConfig[state.mediaType];
  $("#sort-label").textContent = state.mediaType === "photos" ? "拍攝日期（新到舊）" : "錄製日期（新到舊）";
  $$("[data-media]").forEach((button) => {
    const selected = button.dataset.media === state.mediaType;
    button.classList.toggle("active", selected);
    button.setAttribute("aria-selected", String(selected));
    button.tabIndex = selected ? 0 : -1;
  });
  $("#media-panel").setAttribute("aria-labelledby", `${state.mediaType}-tab`);
  $("#media-eyebrow").textContent = config.eyebrow;
  $("#media-title").textContent = config.title;
  $("#all-media-label").textContent = config.title;
  $("#year-filter-label").textContent = config.yearLabel;
  $("#search").placeholder = config.searchPlaceholder;
  $("#search").setAttribute("aria-label", `搜尋${config.label}`);
  $("#search").disabled = false;
  $("#media-actions").hidden = false;
  $("#open-folders").disabled = false;
  $("#open-folders").title = `以年份、專題、Tags${state.mediaType === "photos" ? "與器材" : "與 Codec"}瀏覽${config.label}`;
  $$("[data-indexed-filter]").forEach((section) => { section.hidden = false; });
  $("#photo-metadata-filters").hidden = state.mediaType !== "photos";
  $("#media-metadata-filters").hidden = state.mediaType === "photos";
  $("#media-filter-note").hidden = true;
  $("#all-count").textContent = state.mediaType === "photos" && state.total
    ? new Intl.NumberFormat("zh-TW").format(state.total)
    : state.mediaType === "photos" ? "—" : "0";
  renderFacetsForCurrentMedia();
  updateBatchCopy();
  if (syncURL) {
    const url = new URL(window.location.href);
    if (state.mediaType === "photos") url.searchParams.delete("media");
    else url.searchParams.set("media", state.mediaType);
    window.history.replaceState({}, "", url);
  }
}

function onMediaTabKeydown(event) {
  if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
  event.preventDefault();
  const tabs = $$("[data-media]");
  const current = tabs.indexOf(event.currentTarget);
  let next = event.key === "Home" ? 0 : event.key === "End" ? tabs.length - 1 : current + (event.key === "ArrowRight" ? 1 : -1);
  next = (next + tabs.length) % tabs.length;
  tabs[next].focus();
  selectMedia(tabs[next].dataset.media);
}

function renderActiveFilters() {
  const f = state.filters;
  const values = [];
  if (f.q) values.push(["q", `搜尋：${f.q}`]);
  if (f.year) values.push(["year", f.year]);
  if (f.project) values.push(["project", f.project]);
  f.tags.forEach((tag) => values.push([`tag:${tag}`, `# ${tag}`]));
  if (f.camera) values.push(["camera", f.camera]);
  if (f.lens) values.push(["lens", f.lens]);
  if (f.minISO || f.maxISO) values.push(["iso", `ISO ${f.minISO || "0"}–${f.maxISO || "∞"}`]);
  if (f.location) values.push(["location", "有 GeoTag"]);
  if (f.codec) values.push(["codec", f.codec]);
  if (f.duration) values.push(["duration", {short:"5 分鐘內",medium:"5–30 分鐘",long:"30 分鐘以上"}[f.duration]]);
  if (f.transcript) values.push(["transcript", "有逐字稿"]);
  $("#active-filters").innerHTML = values.map(([key, label]) => `<span class="active-filter">${escapeHTML(label)}<button data-remove="${escapeAttr(key)}" aria-label="移除 ${escapeAttr(label)}">×</button></span>`).join("");
  $("#clear-filters").classList.toggle("visible", values.length > 0);
  $$("[data-remove]", $("#active-filters")).forEach((button) => button.addEventListener("click", () => removeFilter(button.dataset.remove)));
}

function removeFilter(key) {
  if (key.startsWith("tag:")) state.filters.tags = state.filters.tags.filter((tag) => tag !== key.slice(4));
  else if (key === "iso") { state.filters.minISO = ""; state.filters.maxISO = ""; $("#min-iso").value = ""; $("#max-iso").value = ""; }
  else if (key === "location") { state.filters.location = false; $("#location-filter").checked = false; }
  else if (key === "transcript") { state.filters.transcript = false; $("#transcript-filter").checked = false; }
  else { state.filters[key] = ""; const control = $(`#${key}-filter`); if (control) control.value = ""; if (key === "q") $("#search").value = ""; }
  syncFacetUI(); loadPhotos();
}

function clearFilters(reload = true) {
  state.filters = { q: "", year: "", project: "", tags: [], camera: "", lens: "", minISO: "", maxISO: "", location: false, codec: "", duration: "", transcript: false };
  $("#search").value = ""; $("#camera-filter").value = ""; $("#lens-filter").value = "";
  $("#min-iso").value = ""; $("#max-iso").value = ""; $("#location-filter").checked = false;
  $("#codec-filter").value = ""; $("#duration-filter").value = ""; $("#transcript-filter").checked = false;
  syncFacetUI(); closeSidebar(); if (reload) loadPhotos();
}

function hasActiveFilters() {
  const f = state.filters;
  return Boolean(f.q || f.year || f.project || f.tags.length || f.camera || f.lens || f.minISO || f.maxISO || f.location || f.codec || f.duration || f.transcript);
}

function syncFacetUI() {
  $$("[data-kind]").forEach((button) => button.classList.toggle("active", state.filters[button.dataset.kind] === button.dataset.value));
  $$("[data-tag]").forEach((button) => button.classList.toggle("active", state.filters.tags.includes(button.dataset.tag)));
}

function openDetail(id) {
  const photo = state.photos.find((item) => item.id === id);
  if (!photo) return;
  state.selected = photo;
  $("#detail-image").src = photo.imageUrl;
  $("#detail-image").alt = photo.title;
  $("#detail-project").textContent = photo.project;
  $("#detail-title").textContent = photo.title;
  $("#detail-date").textContent = formatDate(photo.takenAt);
  $("#detail-exif").innerHTML = dlItems([["相機", photo.camera], ["鏡頭", photo.lens], ["光圈", photo.aperture], ["快門", photo.shutterSpeed], ["ISO", photo.iso], ["焦距", photo.focalLength]]);
  $("#detail-file").innerHTML = dlItems([["格式", photo.fileType], ["尺寸", photo.dimensions], ["檔案大小", photo.fileSize], ["年份", photo.year]]);
  $("#detail-tags").innerHTML = photo.tags.map((tag) => `<span># ${escapeHTML(tag)}</span>`).join("");
  $("#detail-location-section").hidden = !photo.location;
  if (photo.location) {
    $("#detail-location").textContent = photo.location.name;
    $("#detail-coordinates").textContent = `${photo.location.latitude.toFixed(4)}° N, ${photo.location.longitude.toFixed(4)}° E`;
  }
  const position = state.photos.findIndex((item) => item.id === photo.id);
  $("#detail-position").textContent = `${position + 1} / ${state.photos.length}`;
  if (!dialog.open) dialog.showModal();
}

function navigateDetail(direction) {
  const index = state.photos.findIndex((item) => item.id === state.selected?.id);
  if (index < 0) return;
  const next = (index + direction + state.photos.length) % state.photos.length;
  openDetail(state.photos[next].id);
}

async function showSimilar() {
  if (!state.selected) return;
  const button = $("#similar-button");
  button.disabled = true;
  const oldText = $("strong", button).textContent;
  $("strong", button).textContent = "正在計算向量距離…";
  try {
    const response = await fetch(`/api/v1/photos/${encodeURIComponent(state.selected.id)}/similar?limit=6`);
    if (!response.ok) throw new Error("相似搜尋失敗");
    const data = await response.json();
    $("#similar-title").textContent = "視覺相似照片";
    $("#similar-description").textContent = `以「${state.selected.title}」為基準，依 cosine similarity 排序`;
    $("#similar-grid").innerHTML = data.items.map(({ photo, similarity }) => `
      <button class="similar-card" data-similar-id="${escapeAttr(photo.id)}">
        <img src="${escapeAttr(photo.thumbnailUrl)}" alt="" loading="lazy">
        <span class="similar-copy"><strong>${escapeHTML(photo.title)}</strong><span>${Math.round(similarity * 100)}% 相似</span></span>
      </button>`).join("");
    $$("[data-similar-id]", $("#similar-grid")).forEach((card) => card.addEventListener("click", () => {
      similarDialog.close();
      const match = data.items.find((item) => item.photo.id === card.dataset.similarId)?.photo;
      if (match && !state.photos.some((photo) => photo.id === match.id)) state.photos.push(match);
      openDetail(card.dataset.similarId);
    }));
    dialog.close(); similarDialog.showModal();
  } catch (error) {
    console.error(error);
    $("strong", button).textContent = "相似搜尋暫時無法使用";
    setTimeout(() => { $("strong", button).textContent = oldText; }, 1800);
  } finally { button.disabled = false; $("strong", button).textContent = oldText; }
}

async function openMediaDetail(id) {
  const mediaType = state.mediaType;
  try {
    const response = await fetch(`/api/v1/${mediaType}/${encodeURIComponent(id)}`);
    if (!response.ok) throw new Error(`無法讀取${mediaConfig[mediaType].label}詳情`);
    const asset = await response.json();
    state.selectedMedia = asset;
    $("#media-detail-project").textContent = asset.project;
    $("#media-detail-title").textContent = asset.title;
    $("#media-detail-date").textContent = formatDate(asset.recordedAt);
    $("#media-detail-info").innerHTML = dlItems([
      ["長度", formatDuration(asset.durationMs)], ["Codec", asset.codec], ["格式", asset.mimeType],
      ["尺寸", asset.dimensions], ["取樣率", asset.sampleRate ? `${asset.sampleRate} Hz` : "—"], ["聲道", asset.channels],
    ]);
    $("#media-detail-tags").innerHTML = (asset.tags || []).map((tag) => `<span># ${escapeHTML(tag)}</span>`).join("");
    $("#media-detail-transcript").textContent = asset.transcript || "沒有偵測到可辨識的語音。";
    const visualCount = (asset.segments || []).filter((segment) => segment.segmentType === "visual").length;
    const audioCount = (asset.segments || []).filter((segment) => segment.segmentType === "audio").length;
    $("#media-detail-segments").textContent = `視覺片段 ${visualCount} 個 · 聲音片段 ${audioCount} 個`;
    const video = $("#detail-video");
    const audioWrap = $("#detail-audio-wrap");
    if (mediaType === "videos") {
      audioWrap.hidden = true;
      video.hidden = false;
      video.src = asset.mediaUrl;
      video.load();
    } else {
      video.hidden = true;
      audioWrap.hidden = false;
      $("#detail-audio-image").src = asset.thumbnailUrl;
      $("#detail-audio").src = asset.mediaUrl;
      $("#detail-audio").load();
    }
    $("#media-similar-visual").hidden = mediaType !== "videos";
    $("#media-similar-audio strong").textContent = mediaType === "videos" ? "尋找聲音相似影片" : "尋找聲音相似音訊";
    mediaDialog.showModal();
  } catch (error) {
    console.error(error);
  }
}

async function showSimilarMedia(modality) {
  const asset = state.selectedMedia;
  if (!asset) return;
  const button = modality === "visual" ? $("#media-similar-visual") : $("#media-similar-audio");
  button.disabled = true;
  try {
    const response = await fetch(`/api/v1/${state.mediaType}/${encodeURIComponent(asset.id)}/similar?modality=${modality}&limit=6`);
    if (!response.ok) throw new Error("相似搜尋失敗");
    const data = await response.json();
    const kind = modality === "visual" ? "畫面" : "聲音";
    $("#similar-title").textContent = `${kind}相似${mediaConfig[state.mediaType].label}`;
    $("#similar-description").textContent = `以「${asset.title}」為基準，依 ${modality === "visual" ? "OpenCLIP" : "CLAP"} cosine similarity 排序`;
    $("#similar-grid").innerHTML = data.items.length ? data.items.map(({ asset: match, similarity }) => `
      <button class="similar-card" data-similar-media-id="${escapeAttr(match.id)}">
        <img src="${escapeAttr(match.thumbnailUrl)}" alt="" loading="lazy">
        <span class="similar-copy"><strong>${escapeHTML(match.title)}</strong><span>${Math.round(similarity * 100)}% 相似</span></span>
      </button>`).join("") : `<div class="empty-state"><h2>目前沒有其他可比較的${mediaConfig[state.mediaType].label}</h2><p>再匯入一些內容後即可計算向量距離。</p></div>`;
    $$('[data-similar-media-id]', $("#similar-grid")).forEach((card) => card.addEventListener("click", () => {
      const match = data.items.find((item) => item.asset.id === card.dataset.similarMediaId)?.asset;
      if (match && !state.mediaItems.some((item) => item.id === match.id)) state.mediaItems.push(match);
      similarDialog.close();
      openMediaDetail(card.dataset.similarMediaId);
    }));
    closeMediaDialog();
    similarDialog.showModal();
  } catch (error) {
    console.error(error);
  } finally {
    button.disabled = false;
  }
}

function closeMediaDialog() {
  $("#detail-video").pause();
  $("#detail-audio").pause();
  mediaDialog.close();
}

function formatDuration(milliseconds) {
  const seconds = Math.max(0, Math.round(Number(milliseconds || 0) / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainder = seconds % 60;
  return hours ? `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}` : `${minutes}:${String(remainder).padStart(2, "0")}`;
}

function updateBatchCopy() {
  const config = mediaConfig[state.mediaType];
  $("#batch-title").textContent = `批次整理${config.label}`;
  $("#batch-source-label").textContent = `${config.label}資料夾或 Volume 路徑`;
  $("#batch-auto-label").textContent = state.mediaType === "photos" ? "使用 OpenCLIP 自動加 Tags" : state.mediaType === "videos" ? "使用 OpenCLIP 與 CLAP 自動加 Tags" : "使用 CLAP 自動加 Tags";
  $("#batch-copy").textContent = state.mediaType === "photos" ? "工作會在本機依序執行；關閉視窗不會中止。" : "本機會依序建立片段、逐字稿、向量與 Tags；關閉視窗不會中止。";
}

function dlItems(items) {
  return items.map(([term, value]) => `<div><dt>${escapeHTML(term)}</dt><dd>${escapeHTML(String(value || "—"))}</dd></div>`).join("");
}

function formatDate(value) {
  return new Intl.DateTimeFormat("zh-TW", { year: "numeric", month: "long", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value));
}

function openSidebar() { $("#filter-panel").classList.add("open"); $("#sidebar-scrim").classList.add("visible"); }
function closeSidebar() { $("#filter-panel").classList.remove("open"); $("#sidebar-scrim").classList.remove("visible"); }
function escapeHTML(value) { const el = document.createElement("span"); el.textContent = value ?? ""; return el.innerHTML; }
function escapeAttr(value) { return escapeHTML(value).replaceAll('"', "&quot;"); }

async function openFolderBrowser() {
  const folderDialog = $("#folder-dialog");
  if (!folderDialog.open) folderDialog.showModal();
  $("#folder-sources").innerHTML = `<p class="finder-hint">正在讀取資料夾…</p>`;
  try {
    const response = await fetch(`/api/v1/folders?media=${encodeURIComponent(state.mediaType)}`);
    if (!response.ok) throw new Error("資料夾讀取失敗");
    state.folderTree = await response.json();
    renderFolderSources();
  } catch (error) {
    $("#folder-sources").innerHTML = `<p class="finder-hint">${escapeHTML(error.message)}</p>`;
  }
}

function renderFolderSources() {
  const icon = `<svg viewBox="0 0 24 24"><path d="M3 19V8a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9H3Z"/></svg>`;
  const sources = [...state.folderTree.sources];
  if (state.folderTree.collections?.length) sources.push({ id: "collections", name: "我的資料夾", kind: "collections", children: state.folderTree.collections });
  $("#folder-sources").innerHTML = sources.map((source) => `<button class="finder-row" data-folder-source="${escapeAttr(source.id)}">${icon}<b>${escapeHTML(source.name)}</b><span>›</span></button>`).join("");
  $$("[data-folder-source]").forEach((button) => button.addEventListener("click", () => {
    $$("[data-folder-source]").forEach((item) => item.classList.toggle("active", item === button));
    const source = sources.find((item) => item.id === button.dataset.folderSource);
    renderFolderChildren(source);
  }));
}

function renderFolderChildren(source) {
  const children = source.children || [];
  $("#folder-children").innerHTML = children.length ? children.map((item) => `<button class="finder-row" data-folder-child="${escapeAttr(item.id)}"><svg viewBox="0 0 24 24"><path d="M3 19V8a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9H3Z"/></svg><b>${escapeHTML(item.name)}</b><span>${item.count ?? ""}</span></button>`).join("") : `<p class="finder-hint">這裡還沒有資料夾</p>`;
  $$("[data-folder-child]").forEach((button) => button.addEventListener("click", () => {
    $$("[data-folder-child]").forEach((item) => item.classList.toggle("active", item === button));
    const item = children.find((child) => child.id === button.dataset.folderChild);
    state.selectedFolder = item;
    renderFolderPreview(item);
  }));
}

function renderFolderPreview(folder) {
  const isManual = folder.kind === "manual";
  const config = mediaConfig[state.mediaType];
  $("#folder-preview").innerHTML = `<div class="folder-glyph"><svg viewBox="0 0 48 48"><path d="M5 39V13a4 4 0 0 1 4-4h10l5 5h15a4 4 0 0 1 4 4v21H5Z"/></svg></div><h3>${escapeHTML(folder.name)}</h3><p>${isManual ? "手動收藏的照片，不影響檔案實際位置。" : `這是由資料庫條件即時產生的智慧資料夾${folder.count != null ? `，包含 ${folder.count} ${config.unit}${config.label}` : ""}。`}</p><button id="browse-selected-folder">瀏覽${config.label}</button>`;
  $("#browse-selected-folder").addEventListener("click", () => browseFolder(folder));
}

async function browseFolder(folder) {
  $("#folder-dialog").close();
  if (folder.kind === "manual") {
    renderSkeletons();
    const response = await fetch(`/api/v1/collections/${encodeURIComponent(folder.id)}/photos`);
    if (!response.ok) return;
    const page = await response.json();
    state.photos = page.items || []; state.total = page.total || 0;
    renderPhotos();
    $("#result-summary").textContent = `${folder.name} · ${state.total} 張照片`;
    return;
  }
  clearFilters(false);
  const filter = folder.filter || {};
  if (filter.year) state.filters.year = String(filter.year);
  if (filter.project) state.filters.project = filter.project;
  if (filter.tag) state.filters.tags = [filter.tag];
  if (Array.isArray(filter.tags)) state.filters.tags = filter.tags;
  if (filter.camera) state.filters.camera = filter.camera;
  if (filter.lens) state.filters.lens = filter.lens;
  if (filter.codec) state.filters.codec = filter.codec;
  if (filter.q) { state.filters.q = filter.q; $("#search").value = filter.q; }
  if (filter.min_iso) { state.filters.minISO = String(filter.min_iso); $("#min-iso").value = filter.min_iso; }
  if (filter.max_iso) { state.filters.maxISO = String(filter.max_iso); $("#max-iso").value = filter.max_iso; }
  if (filter.has_location) { state.filters.location = true; $("#location-filter").checked = true; }
  syncFacetUI();
  await loadPhotos();
}

async function submitBatch(event) {
  event.preventDefault();
  const form = new FormData(event.currentTarget);
  const payload = {
    sourceRoot: form.get("sourceRoot"), project: form.get("project"),
    tags: String(form.get("tags") || "").split(",").map((tag) => tag.trim()).filter(Boolean),
    recursive: form.get("recursive") === "on", autoTags: form.get("autoTags") === "on",
    mediaTypes: [mediaConfig[state.mediaType].singular],
  };
  const submit = $(".batch-submit"); submit.disabled = true; submit.firstChild.textContent = "正在建立工作… ";
  try {
    const response = await fetch("/api/v1/batch-jobs", { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify(payload) });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || "無法建立批次工作");
    state.batchJobID = data.id;
    $("#batch-form").hidden = true; $("#batch-progress").hidden = false;
    updateBatchProgress(data); followBatch(data.id);
  } catch (error) {
    alert(error.message);
  } finally { submit.disabled = false; submit.firstChild.textContent = "建立批次工作 "; }
}

function followBatch(id) {
  if (state.batchEvents) state.batchEvents.close();
  const events = new EventSource(`/api/v1/batch-jobs/${encodeURIComponent(id)}/events`);
  state.batchEvents = events;
  events.addEventListener("progress", (event) => updateBatchProgress(JSON.parse(event.data)));
  events.addEventListener("complete", (event) => { updateBatchProgress(JSON.parse(event.data)); events.close(); state.batchEvents = null; loadFacets(); loadPhotos(); });
  events.onerror = () => { events.close(); state.batchEvents = null; pollBatch(id); };
}

async function pollBatch(id) {
  try {
    const response = await fetch(`/api/v1/batch-jobs/${encodeURIComponent(id)}`);
    if (!response.ok) return;
    const job = await response.json(); updateBatchProgress(job);
    if (!["completed","completed_with_errors","failed","cancelled"].includes(job.status)) setTimeout(() => pollBatch(id), 1500);
  } catch (_) { setTimeout(() => pollBatch(id), 2500); }
}

function updateBatchProgress(job) {
  const labels = {pending:"等待處理",scanning:"掃描媒體檔案",running:"建立縮圖、片段、逐字稿與向量",completed:"處理完成",completed_with_errors:"完成，部分檔案失敗",failed:"工作失敗",cancelled:"已停止"};
  const percent = job.discoveredCount ? Math.round(job.processedCount / job.discoveredCount * 100) : 0;
  $("#batch-status").textContent = labels[job.status] || job.status; $("#batch-percent").textContent = `${percent}%`;
  $("#batch-progress-bar").style.width = `${percent}%`; $("#batch-discovered").textContent = job.discoveredCount;
  $("#batch-success").textContent = job.succeededCount; $("#batch-failed").textContent = job.failedCount;
  $("#batch-current").textContent = job.error || job.currentPath || (job.status === "pending" ? "工作已存入 PostgreSQL queue" : "");
  $("#batch-cancel").hidden = ["completed","completed_with_errors","failed","cancelled"].includes(job.status);
}

async function cancelBatch() {
  if (!state.batchJobID) return;
  await fetch(`/api/v1/batch-jobs/${encodeURIComponent(state.batchJobID)}/cancel`, {method:"POST"});
  $("#batch-cancel").disabled = true; $("#batch-cancel").textContent = "正在停止…";
}
