const { t, formatNumber, formatDate: formatLocalizedDate, applyTranslations } = window.ApoFocusI18n;

const mediaConfig = {
  photos: { eyebrow: "PHOTO ARCHIVE", singular: "photo" },
  videos: { eyebrow: "VIDEO ARCHIVE", singular: "video" },
  audios: { eyebrow: "AUDIO ARCHIVE", singular: "audio" },
};

const mediaText = (mediaType, key, params) => t(`media.${mediaType}.${key}`, params);

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
  similar: null,
  folderTree: null,
  selectedFolderSource: null,
  selectedFolder: null,
  batchJobID: null,
  batchJob: null,
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
	$("#photo-edit-open").addEventListener("click", openPhotoEditor);
	$("#media-edit-open").addEventListener("click", openMediaEditor);
	$("#photo-edit-form").addEventListener("submit", savePhotoEdit);
	$("#media-edit-form").addEventListener("submit", saveMediaEdit);
	$$('[data-edit-cancel]').forEach((button) => button.addEventListener("click", () => closeEditor(button.dataset.editCancel)));
  window.addEventListener("apofocus:localechange", refreshLocalizedUI);
}

function refreshLocalizedUI() {
  applyTranslations();
  applyMediaUI({ syncURL: false });
  renderActiveFilters();
  if (state.mediaType === "photos") renderPhotos();
  else renderMediaAssets();
  if (state.selected && dialog.open) openDetail(state.selected.id);
  if (state.selectedMedia && mediaDialog.open) renderMediaDetail(state.selectedMedia, state.mediaType);
  if (state.similar && similarDialog.open) renderSimilarResults();
  if (state.folderTree) {
    renderFolderSources();
    if (state.selectedFolderSource) renderFolderChildren(state.selectedFolderSource);
    if (state.selectedFolder) renderFolderPreview(state.selectedFolder);
  }
  if (state.batchJob) updateBatchProgress(state.batchJob);
}

async function loadFacets() {
  const mediaType = state.mediaType;
  try {
    const endpoint = mediaType === "photos" ? "/api/v1/facets" : `/api/v1/${mediaType}/facets`;
    const response = await fetch(endpoint);
    if (!response.ok) throw new Error(t("filters.unavailable"));
    const facets = await response.json();
    if (mediaType === "photos") state.facets = facets;
    else state.mediaFacets[mediaType] = facets;
    if (state.mediaType === mediaType) renderFacetsForCurrentMedia();
  } catch (error) {
    console.error(error);
    $("#year-filters").innerHTML = `<p class="result-summary">${escapeHTML(t("filters.unavailable"))}</p>`;
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
    if (!response.ok) throw new Error(t("media.loadError", { media: mediaText("photos", "label") }));
    const page = await response.json();
    state.photos = page.items || [];
    state.total = page.total || 0;
    renderPhotos();
    renderActiveFilters();
    if (!f.q && !f.year && !f.project && !f.tags.length && !f.camera && !f.lens && !f.minISO && !f.maxISO && !f.location) {
      $("#all-count").textContent = formatNumber(state.total);
      $("#photos-tab-count").textContent = formatNumber(state.total);
    }
  } catch (error) {
    console.error(error);
    grid.innerHTML = renderLoadError("photos");
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
    if (!response.ok) throw new Error(t("media.loadError", { media: mediaText(mediaType, "label") }));
    const page = await response.json();
    if (state.mediaType !== mediaType) return;
    state.mediaItems = page.items || [];
    state.total = page.total || 0;
    renderMediaAssets();
    renderActiveFilters();
    const count = formatNumber(state.total);
    $(`#${mediaType}-tab-count`).textContent = count;
    $("#all-count").textContent = count;
  } catch (error) {
    console.error(error);
    grid.innerHTML = renderLoadError(mediaType);
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
  $("#year-filters").innerHTML = (facets.years || []).map((item) => facetButton("year", item)).join("") || `<p class="facet-empty">${escapeHTML(t("filters.noYears"))}</p>`;
  $("#project-filters").innerHTML = (facets.projects || []).map((item) => facetButton("project", item)).join("") || `<p class="facet-empty">${escapeHTML(t("filters.noProjects"))}</p>`;
  $("#tag-filters").innerHTML = (facets.tags || []).slice(0, 12).map((item) => `<button class="tag-pill" data-tag="${escapeAttr(item.value)}">${escapeHTML(item.value)} <span>${formatNumber(item.count)}</span></button>`).join("") || `<p class="facet-empty">${escapeHTML(t("filters.noTags"))}</p>`;
  if (state.mediaType === "photos") {
    resetFacetSelect($("#camera-filter"), t("filters.allCameras"));
    resetFacetSelect($("#lens-filter"), t("filters.allLenses"));
    fillSelect($("#camera-filter"), facets.cameras || []);
    fillSelect($("#lens-filter"), facets.lenses || []);
    $("#camera-filter").value = state.filters.camera;
    $("#lens-filter").value = state.filters.lens;
  } else {
    resetFacetSelect($("#codec-filter"), t("filters.allCodecs"));
    fillSelect($("#codec-filter"), facets.codecs || []);
    $("#codec-filter").value = state.filters.codec;
  }
  syncFacetUI();
}

function resetFacetSelect(select, label) {
  select.innerHTML = `<option value="">${label}</option>`;
}

function facetButton(kind, item) {
  return `<button class="facet-row" data-kind="${kind}" data-value="${escapeAttr(item.value)}"><span>${escapeHTML(item.value)}</span><span class="count">${formatNumber(item.count)}</span></button>`;
}

function fillSelect(select, items) {
  items.forEach((item) => select.insertAdjacentHTML("beforeend", `<option value="${escapeAttr(item.value)}">${escapeHTML(item.value)} (${formatNumber(item.count)})</option>`));
}

function renderLoadError(mediaType) {
  return `<div class="empty-state"><h2>${escapeHTML(t("media.loadError", { media: mediaText(mediaType, "label") }))}</h2><p>${escapeHTML(t("media.loadErrorCopy"))}</p></div>`;
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
  $("#result-summary").textContent = mediaText("photos", "results", { count: formatNumber(state.total) });
  renderPhotoEmptyState();
  $("#empty-state").hidden = state.photos.length > 0;
  grid.hidden = state.photos.length === 0;
  grid.innerHTML = state.photos.map((photo) => `
    <button class="photo-card ${photo.aspectRatio === "portrait" ? "portrait" : ""} ${photo.availabilityStatus && photo.availabilityStatus !== "available" && photo.availabilityStatus !== "unknown" ? "unavailable" : ""}" data-photo-id="${escapeAttr(photo.id)}" style="background:${escapeAttr(photo.dominantColor)}" aria-label="${escapeAttr(mediaText("photos", "view", { title: photo.title }))}">
	  ${photo.thumbnailUrl || photo.imageUrl ? `<img src="${escapeAttr(photo.thumbnailUrl || photo.imageUrl)}" alt="" loading="lazy" decoding="async">` : `<span class="catalog-placeholder">PHOTO</span>`}
      <span class="card-badge">${escapeHTML(availabilityLabel(photo.availabilityStatus) || photo.fileType)}</span>
      <span class="card-copy"><h3>${escapeHTML(photo.title)}</h3><p>${escapeHTML(photo.project)} · ${photo.year}</p></span>
    </button>`).join("");
  $$(".photo-card", grid).forEach((card) => card.addEventListener("click", () => openDetail(card.dataset.photoId)));
}

function renderPhotoEmptyState() {
  $("#empty-symbol").textContent = "0";
  const filtered = hasActiveFilters();
  $("#empty-title").textContent = mediaText("photos", filtered ? "emptyFiltered" : "emptyInitial");
  $("#empty-copy").textContent = filtered ? t("media.emptyFilteredCopy") : mediaText("photos", "emptyInitialCopy");
  $("#empty-clear").hidden = !filtered;
}

function mediaPreview(asset, mediaType) {
  if (mediaType === "audios") {
    return `<span class="audio-artwork" aria-hidden="true"><strong>♪</strong><small>${escapeHTML(asset.codec || "AUDIO")}</small></span>`;
  }
  return asset.thumbnailUrl ? `<img src="${escapeAttr(asset.thumbnailUrl)}" alt="" loading="lazy" decoding="async">` : `<span class="catalog-placeholder">VIDEO</span>`;
}

function renderMediaAssets() {
  const mediaType = state.mediaType;
  $("#result-summary").textContent = mediaText(mediaType, "results", { count: formatNumber(state.total) });
  const filtered = hasActiveFilters();
  $("#empty-symbol").textContent = mediaType === "videos" ? "▶" : "♪";
  $("#empty-title").textContent = mediaText(mediaType, filtered ? "emptyFiltered" : "emptyInitial");
  $("#empty-copy").textContent = filtered ? t("media.emptyFilteredCopy") : mediaText(mediaType, "emptyInitialCopy");
  $("#empty-clear").hidden = !filtered;
  $("#empty-state").hidden = state.mediaItems.length > 0;
  grid.hidden = state.mediaItems.length === 0;
  grid.innerHTML = state.mediaItems.map((asset) => `
    <button class="photo-card media-card ${asset.availabilityStatus && asset.availabilityStatus !== "available" && asset.availabilityStatus !== "unknown" ? "unavailable" : ""}" data-media-id="${escapeAttr(asset.id)}" aria-label="${escapeAttr(mediaText(mediaType, "view", { title: asset.title }))}">
      ${mediaPreview(asset, mediaType)}
      <span class="media-kind">${mediaType === "videos" ? "▶" : "♪"}</span>
      <span class="card-badge">${escapeHTML(availabilityLabel(asset.availabilityStatus) || formatDuration(asset.durationMs))}</span>
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
  $("#sort-label").textContent = t(state.mediaType === "photos" ? "media.sort.photo" : "media.sort.recording");
  $$("[data-media]").forEach((button) => {
    const selected = button.dataset.media === state.mediaType;
    button.classList.toggle("active", selected);
    button.setAttribute("aria-selected", String(selected));
    button.tabIndex = selected ? 0 : -1;
  });
  $("#media-panel").setAttribute("aria-labelledby", `${state.mediaType}-tab`);
  $("#media-eyebrow").textContent = config.eyebrow;
  $("#media-title").textContent = mediaText(state.mediaType, "all");
  $("#all-media-label").textContent = mediaText(state.mediaType, "all");
  $("#year-filter-label").textContent = mediaText(state.mediaType, "year");
  $("#search").placeholder = mediaText(state.mediaType, "searchPlaceholder");
  $("#search").setAttribute("aria-label", mediaText(state.mediaType, "searchAria"));
  $("#search").disabled = false;
  $("#media-actions").hidden = false;
  $("#open-folders").disabled = false;
  $("#open-folders").title = t(`nav.folderTitle.${state.mediaType}`);
  $$("[data-indexed-filter]").forEach((section) => { section.hidden = false; });
  $("#photo-metadata-filters").hidden = state.mediaType !== "photos";
  $("#media-metadata-filters").hidden = state.mediaType === "photos";
  $("#media-filter-note").hidden = true;
  $("#all-count").textContent = state.mediaType === "photos" && state.total
    ? formatNumber(state.total)
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
  if (f.q) values.push(["q", t("filters.search", { query: f.q })]);
  if (f.year) values.push(["year", f.year]);
  if (f.project) values.push(["project", f.project]);
  f.tags.forEach((tag) => values.push([`tag:${tag}`, `# ${tag}`]));
  if (f.camera) values.push(["camera", f.camera]);
  if (f.lens) values.push(["lens", f.lens]);
  if (f.minISO || f.maxISO) values.push(["iso", `ISO ${f.minISO || "0"}–${f.maxISO || "∞"}`]);
  if (f.location) values.push(["location", t("filters.hasGeo")]);
  if (f.codec) values.push(["codec", f.codec]);
  if (f.duration) values.push(["duration", t(`filters.duration.${f.duration}`)]);
  if (f.transcript) values.push(["transcript", t("filters.hasTranscript")]);
  $("#active-filters").innerHTML = values.map(([key, label]) => `<span class="active-filter">${escapeHTML(label)}<button data-remove="${escapeAttr(key)}" aria-label="${escapeAttr(t("common.remove", { label }))}">×</button></span>`).join("");
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
	closeEditor("photo");
  $("#detail-image").src = photo.availabilityStatus === "available" || photo.availabilityStatus === "unknown" ? photo.imageUrl : photo.thumbnailUrl;
  $("#detail-image").alt = photo.title;
  $("#detail-project").textContent = photo.project;
  $("#detail-title").textContent = photo.title;
  $("#detail-date").textContent = formatDate(photo.takenAt);
  $("#detail-exif").innerHTML = dlItems([[t("field.camera"), photo.camera], [t("field.lens"), photo.lens], [t("field.aperture"), photo.aperture], [t("field.shutter"), photo.shutterSpeed], ["ISO", photo.iso], [t("field.focalLength"), photo.focalLength]]);
  $("#detail-file").innerHTML = dlItems([[t("field.format"), photo.fileType], [t("field.dimensions"), photo.dimensions], [t("field.fileSize"), photo.fileSize], [t("field.year"), photo.year], [t("field.originalStatus"), availabilityLabel(photo.availabilityStatus, true)], [t("field.thumbnailStatus"), availabilityLabel(photo.thumbnailStatus, true)]]);
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
  $("strong", button).textContent = t("similar.calculating");
  try {
    const response = await fetch(`/api/v1/photos/${encodeURIComponent(state.selected.id)}/similar?limit=6`);
    if (!response.ok) throw new Error(t("similar.failed"));
    const data = await response.json();
    state.similar = { kind: "photo", source: state.selected, items: data.items };
    renderSimilarResults();
    dialog.close(); similarDialog.showModal();
  } catch (error) {
    console.error(error);
    $("strong", button).textContent = t("similar.failed");
    setTimeout(() => { $("strong", button).textContent = oldText; }, 1800);
  } finally { button.disabled = false; $("strong", button).textContent = oldText; }
}

async function openMediaDetail(id) {
  const mediaType = state.mediaType;
  try {
    const response = await fetch(`/api/v1/${mediaType}/${encodeURIComponent(id)}`);
    if (!response.ok) throw new Error(t("media.loadError", { media: mediaText(mediaType, "label") }));
    const asset = await response.json();
    state.selectedMedia = asset;
    renderMediaDetail(asset, mediaType, { loadPlayer: true });
    mediaDialog.showModal();
  } catch (error) {
    console.error(error);
  }
}

function renderMediaDetail(asset, mediaType, { loadPlayer = false } = {}) {
	closeEditor("media");
  $("#media-detail-project").textContent = asset.project;
  $("#media-detail-title").textContent = asset.title;
  $("#media-detail-date").textContent = formatDate(asset.recordedAt);
  const mediaInfo = [
    [t("field.duration"), formatDuration(asset.durationMs)], ["Codec", asset.codec], [t("field.format"), asset.mimeType],
    [t("field.dimensions"), asset.dimensions], [t("field.sampleRate"), asset.sampleRate ? `${formatNumber(asset.sampleRate)} Hz` : "—"], [t("field.channels"), asset.channels],
    [t("field.originalStatus"), availabilityLabel(asset.availabilityStatus, true)],
  ];
  if (mediaType === "videos") mediaInfo.push([t("field.previewStatus"), availabilityLabel(asset.thumbnailStatus, true)]);
  $("#media-detail-info").innerHTML = dlItems(mediaInfo);
  $("#media-detail-tags").innerHTML = (asset.tags || []).map((tag) => `<span># ${escapeHTML(tag)}</span>`).join("");
  $("#media-detail-transcript").textContent = asset.transcript || t("media.noTranscript");
  const visualCount = (asset.segments || []).filter((segment) => segment.segmentType === "visual").length;
  const audioCount = (asset.segments || []).filter((segment) => segment.segmentType === "audio").length;
  $("#media-detail-segments").textContent = t("media.segmentSummary", { visual: formatNumber(visualCount), audio: formatNumber(audioCount) });
  const video = $("#detail-video");
  const audioWrap = $("#detail-audio-wrap");
  if (mediaType === "videos") {
    audioWrap.hidden = true;
    video.hidden = false;
    if (loadPlayer) { video.src = asset.mediaUrl; video.load(); }
  } else {
    video.hidden = true;
    audioWrap.hidden = false;
    if (loadPlayer) { $("#detail-audio").src = asset.mediaUrl; $("#detail-audio").load(); }
  }
  $("#media-similar-visual").hidden = mediaType !== "videos";
  $("#media-similar-audio strong").textContent = t(mediaType === "videos" ? "similar.audioVideoAction" : "similar.audioAudioAction");
}

function openPhotoEditor() {
	const photo = state.selected; if (!photo) return;
	const form = $("#photo-edit-form");
	form.hidden = false; $("#photo-edit-open").hidden = true;
	setFormValue(form,"title",photo.title); setFormValue(form,"project",photo.project);
	setFormValue(form,"takenAt",localDateTime(photo.takenAt)); setFormValue(form,"tags",(photo.tags||[]).join(", "));
	for (const name of ["camera","lens","aperture","shutterSpeed","iso","focalLength","description","copyright","rating"]) setFormValue(form,name,photo[name] ?? "");
	setFormValue(form,"locationName",photo.location?.name||""); setFormValue(form,"latitude",photo.location?.latitude??""); setFormValue(form,"longitude",photo.location?.longitude??"");
	form.elements.favorite.checked=Boolean(photo.favorite); form.elements.clearLocation.checked=false;
	setFormValue(form,"userMetadata",JSON.stringify(photo.userMetadata||{},null,2));
}

function openMediaEditor() {
	const asset=state.selectedMedia; if(!asset)return;
	const form=$("#media-edit-form"); form.hidden=false; $("#media-edit-open").hidden=true;
	for(const name of ["title","project","description","copyright","rating","transcript"]) setFormValue(form,name,asset[name]??"");
	setFormValue(form,"recordedAt",localDateTime(asset.recordedAt)); setFormValue(form,"tags",(asset.tags||[]).join(", "));
	form.elements.favorite.checked=Boolean(asset.favorite); setFormValue(form,"userMetadata",JSON.stringify(asset.userMetadata||{},null,2));
}

function closeEditor(kind) {
	const form=$(`#${kind}-edit-form`),button=$(`#${kind}-edit-open`); if(form)form.hidden=true;if(button)button.hidden=false;
}

async function savePhotoEdit(event) {
	event.preventDefault(); const form=event.currentTarget, photo=state.selected; if(!photo)return;
	try {
		const location=coordinatesFrom(form), userMetadata=parseMetadata(form.elements.userMetadata.value);
		const payload={title:valueOf(form,"title"),project:valueOf(form,"project"),takenAt:isoDate(form,"takenAt"),tags:tagsOf(form),camera:valueOf(form,"camera"),lens:valueOf(form,"lens"),aperture:valueOf(form,"aperture"),shutterSpeed:valueOf(form,"shutterSpeed"),iso:Number(valueOf(form,"iso")||0),focalLength:valueOf(form,"focalLength"),description:valueOf(form,"description"),copyright:valueOf(form,"copyright"),rating:Number(valueOf(form,"rating")||0),favorite:form.elements.favorite.checked,userMetadata,revision:photo.revision||0,clearLocation:form.elements.clearLocation.checked};
		if(location)payload.location=location;
		const updated=await patchAsset(`/api/v1/photos/${encodeURIComponent(photo.id)}`,payload);
		state.photos=state.photos.map((item)=>item.id===updated.id?updated:item); state.selected=updated; renderPhotos(); openDetail(updated.id); await loadFacets();
	} catch(error){ showEditError(form,error); }
}

async function saveMediaEdit(event) {
	event.preventDefault(); const form=event.currentTarget,asset=state.selectedMedia;if(!asset)return;
	try {
		const payload={title:valueOf(form,"title"),project:valueOf(form,"project"),recordedAt:isoDate(form,"recordedAt"),tags:tagsOf(form),description:valueOf(form,"description"),copyright:valueOf(form,"copyright"),rating:Number(valueOf(form,"rating")||0),favorite:form.elements.favorite.checked,transcript:valueOf(form,"transcript"),userMetadata:parseMetadata(form.elements.userMetadata.value),revision:asset.revision||0};
		const updated=await patchAsset(`/api/v1/${state.mediaType}/${encodeURIComponent(asset.id)}`,payload);
		state.selectedMedia=updated; state.mediaItems=state.mediaItems.map((item)=>item.id===updated.id?{...item,...updated}:item); renderMediaAssets(); renderMediaDetail(updated,state.mediaType,{loadPlayer:false}); await loadFacets();
	} catch(error){showEditError(form,error);}
}

async function patchAsset(url,payload){const response=await fetch(url,{method:"PATCH",headers:{"Content-Type":"application/json"},body:JSON.stringify(payload)});const body=await response.json();if(!response.ok)throw new Error(body.error||t("edit.failed"));return body;}
function setFormValue(form,name,value){if(form.elements[name])form.elements[name].value=value??"";}
function valueOf(form,name){return String(form.elements[name]?.value||"").trim();}
function tagsOf(form){return valueOf(form,"tags").split(",").map((value)=>value.trim()).filter(Boolean);}
function isoDate(form,name){const value=valueOf(form,name);return value?new Date(value).toISOString():undefined;}
function localDateTime(value){if(!value)return"";const date=new Date(value),offset=date.getTimezoneOffset()*60000;return new Date(date-offset).toISOString().slice(0,16);}
function coordinatesFrom(form){const lat=valueOf(form,"latitude"),lng=valueOf(form,"longitude");if(lat===""||lng==="")return null;return{name:valueOf(form,"locationName"),latitude:Number(lat),longitude:Number(lng)};}
function parseMetadata(value){try{return JSON.parse(value||"{}");}catch{throw new Error(t("edit.invalidJSON"));}}
function showEditError(form,error){const target=$("[data-edit-message]",form);target.textContent=error.message||t("edit.failed");}

async function showSimilarMedia(modality) {
  const asset = state.selectedMedia;
  if (!asset) return;
  const button = modality === "visual" ? $("#media-similar-visual") : $("#media-similar-audio");
  button.disabled = true;
  try {
    const response = await fetch(`/api/v1/${state.mediaType}/${encodeURIComponent(asset.id)}/similar?modality=${modality}&limit=6`);
    if (!response.ok) throw new Error(t("similar.failed"));
    const data = await response.json();
    state.similar = { kind: "media", modality, mediaType: state.mediaType, source: asset, items: data.items };
    renderSimilarResults();
    closeMediaDialog();
    similarDialog.showModal();
  } catch (error) {
    console.error(error);
  } finally {
    button.disabled = false;
  }
}

function renderSimilarResults() {
  const similar = state.similar;
  if (!similar) return;
  if (similar.kind === "photo") {
    $("#similar-title").textContent = t("similar.photoTitle");
    $("#similar-description").textContent = t("similar.description", { title: similar.source.title, model: "pgvector" });
    $("#similar-grid").innerHTML = similar.items.map(({ photo, similarity }) => `
      <button class="similar-card" data-similar-id="${escapeAttr(photo.id)}">
        <img src="${escapeAttr(photo.thumbnailUrl)}" alt="" loading="lazy">
        <span class="similar-copy"><strong>${escapeHTML(photo.title)}</strong><span>${escapeHTML(t("similar.score", { score: formatNumber(Math.round(similarity * 100)) }))}</span></span>
      </button>`).join("");
    $$("[data-similar-id]", $("#similar-grid")).forEach((card) => card.addEventListener("click", () => {
      similarDialog.close();
      const match = similar.items.find((item) => item.photo.id === card.dataset.similarId)?.photo;
      if (match && !state.photos.some((photo) => photo.id === match.id)) state.photos.push(match);
      openDetail(card.dataset.similarId);
    }));
    return;
  }
  $("#similar-title").textContent = t(`similar.${similar.modality}Title.${similar.mediaType}`);
  $("#similar-description").textContent = t("similar.description", { title: similar.source.title, model: similar.modality === "visual" ? "OpenCLIP" : "CLAP" });
  $("#similar-grid").innerHTML = similar.items.length ? similar.items.map(({ asset: match, similarity }) => `
    <button class="similar-card" data-similar-media-id="${escapeAttr(match.id)}">
      ${mediaPreview(match, similar.mediaType)}
      <span class="similar-copy"><strong>${escapeHTML(match.title)}</strong><span>${escapeHTML(t("similar.score", { score: formatNumber(Math.round(similarity * 100)) }))}</span></span>
    </button>`).join("") : `<div class="empty-state"><h2>${escapeHTML(t("similar.noComparable", { media: mediaText(similar.mediaType, "label") }))}</h2><p>${escapeHTML(t("similar.noComparableCopy"))}</p></div>`;
  $$('[data-similar-media-id]', $("#similar-grid")).forEach((card) => card.addEventListener("click", () => {
    const match = similar.items.find((item) => item.asset.id === card.dataset.similarMediaId)?.asset;
    if (match && !state.mediaItems.some((item) => item.id === match.id)) state.mediaItems.push(match);
    similarDialog.close();
    openMediaDetail(card.dataset.similarMediaId);
  }));
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

function availabilityLabel(status, includeAvailable = false) {
  return status === "available" && !includeAvailable ? "" : (status ? t(`status.${status}`) : "");
}

function updateBatchCopy() {
  const media = mediaText(state.mediaType, "label");
  $("#batch-title").textContent = t("batch.title", { media });
  $("#batch-source-label").textContent = t("batch.source", { media });
  $("#batch-auto-label").textContent = t(`batch.auto.${state.mediaType}`);
  $("#batch-copy").textContent = t(state.mediaType === "photos" ? "batch.copy.photos" : "batch.copy.media");
}

function dlItems(items) {
  return items.map(([term, value]) => `<div><dt>${escapeHTML(term)}</dt><dd>${escapeHTML(String(value || "—"))}</dd></div>`).join("");
}

function formatDate(value) {
  return formatLocalizedDate(value);
}

function openSidebar() { $("#filter-panel").classList.add("open"); $("#sidebar-scrim").classList.add("visible"); }
function closeSidebar() { $("#filter-panel").classList.remove("open"); $("#sidebar-scrim").classList.remove("visible"); }
function escapeHTML(value) { const el = document.createElement("span"); el.textContent = value ?? ""; return el.innerHTML; }
function escapeAttr(value) { return escapeHTML(value).replaceAll('"', "&quot;"); }

async function openFolderBrowser() {
  const folderDialog = $("#folder-dialog");
  if (!folderDialog.open) folderDialog.showModal();
  $("#folder-sources").innerHTML = `<p class="finder-hint">${escapeHTML(t("folder.loading"))}</p>`;
  try {
    const response = await fetch(`/api/v1/folders?media=${encodeURIComponent(state.mediaType)}`);
    if (!response.ok) throw new Error(t("folder.loadError"));
    state.folderTree = await response.json();
    renderFolderSources();
  } catch (error) {
    $("#folder-sources").innerHTML = `<p class="finder-hint">${escapeHTML(error.message)}</p>`;
  }
}

function renderFolderSources() {
  const icon = `<svg viewBox="0 0 24 24"><path d="M3 19V8a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9H3Z"/></svg>`;
  const sources = [...state.folderTree.sources];
  if (state.folderTree.collections?.length) sources.push({ id: "collections", name: t("folder.myFolders"), kind: "collections", children: state.folderTree.collections });
  $("#folder-sources").innerHTML = sources.map((source) => `<button class="finder-row ${source.id === state.selectedFolderSource?.id ? "active" : ""}" data-folder-source="${escapeAttr(source.id)}">${icon}<b>${escapeHTML(folderSourceName(source))}</b><span>›</span></button>`).join("");
  $$("[data-folder-source]").forEach((button) => button.addEventListener("click", () => {
    $$("[data-folder-source]").forEach((item) => item.classList.toggle("active", item === button));
    const source = sources.find((item) => item.id === button.dataset.folderSource);
    renderFolderChildren(source);
  }));
}

function renderFolderChildren(source) {
  state.selectedFolderSource = source;
  const children = source.children || [];
  $("#folder-children").innerHTML = children.length ? children.map((item) => `<button class="finder-row" data-folder-child="${escapeAttr(item.id)}"><svg viewBox="0 0 24 24"><path d="M3 19V8a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v9H3Z"/></svg><b>${escapeHTML(item.name)}</b><span>${item.count == null ? "" : formatNumber(item.count)}</span></button>`).join("") : `<p class="finder-hint">${escapeHTML(t("folder.empty"))}</p>`;
  $$("[data-folder-child]").forEach((button) => button.addEventListener("click", () => {
    $$("[data-folder-child]").forEach((item) => item.classList.toggle("active", item === button));
    const item = children.find((child) => child.id === button.dataset.folderChild);
    state.selectedFolder = item;
    renderFolderPreview(item);
  }));
}

function folderSourceName(source) {
  if (source.id === "years") return mediaText(state.mediaType, "year");
  if (source.id === "projects") return t("filters.project");
  if (source.id === "tags") return t("filters.tags");
  if (source.id === "cameras") return t("filters.camera");
  if (source.id === "lenses") return t("filters.lens");
  if (source.id === "codecs") return t("filters.codec");
  return source.name;
}

function renderFolderPreview(folder) {
  const isManual = folder.kind === "manual";
  const count = folder.count == null ? null : mediaText(state.mediaType, "count", { count: formatNumber(folder.count) });
  const copy = isManual ? t("folder.manualCopy") : count ? t("folder.smartCopyCount", { count }) : t("folder.smartCopy");
  $("#folder-preview").innerHTML = `<div class="folder-glyph"><svg viewBox="0 0 48 48"><path d="M5 39V13a4 4 0 0 1 4-4h10l5 5h15a4 4 0 0 1 4 4v21H5Z"/></svg></div><h3>${escapeHTML(folder.name)}</h3><p>${escapeHTML(copy)}</p><button id="browse-selected-folder">${escapeHTML(t("folder.browse", { media: mediaText(state.mediaType, "label") }))}</button>`;
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
    $("#result-summary").textContent = t("folder.result", { name: folder.name, count: mediaText("photos", "count", { count: formatNumber(state.total) }) });
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
  const submit = $(".batch-submit"); submit.disabled = true; $(".batch-submit-label", submit).textContent = t("batch.creating");
  try {
    const response = await fetch("/api/v1/batch-jobs", { method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify(payload) });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || t("batch.createError"));
    state.batchJobID = data.id;
    $("#batch-form").hidden = true; $("#batch-progress").hidden = false;
    updateBatchProgress(data); followBatch(data.id);
  } catch (error) {
    alert(error.message);
  } finally { submit.disabled = false; $(".batch-submit-label", submit).textContent = t("batch.create"); }
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
  state.batchJob = job;
  const percent = job.discoveredCount ? Math.round(job.processedCount / job.discoveredCount * 100) : 0;
  $("#batch-status").textContent = t(`batch.status.${job.status}`); $("#batch-percent").textContent = `${formatNumber(percent)}%`;
  $("#batch-progress-bar").style.width = `${percent}%`; $("#batch-discovered").textContent = formatNumber(job.discoveredCount);
  $("#batch-success").textContent = formatNumber(job.succeededCount); $("#batch-failed").textContent = formatNumber(job.failedCount);
  $("#batch-current").textContent = job.error || job.currentPath || (job.status === "pending" ? t("batch.queue") : "");
  $("#batch-cancel").hidden = ["completed","completed_with_errors","failed","cancelled"].includes(job.status);
}

async function cancelBatch() {
  if (!state.batchJobID) return;
  await fetch(`/api/v1/batch-jobs/${encodeURIComponent(state.batchJobID)}/cancel`, {method:"POST"});
  $("#batch-cancel").disabled = true; $("#batch-cancel").textContent = t("batch.cancelling");
}
