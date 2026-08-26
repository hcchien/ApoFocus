const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const i18n = require("./static/i18n.js");

test("all locales expose the same translation keys", () => {
  const reference = Object.keys(i18n.translations["zh-TW"]).sort();
  for (const locale of i18n.supportedLocales) {
    assert.deepEqual(Object.keys(i18n.translations[locale]).sort(), reference, locale);
  }
});

test("locale variants resolve to supported locales", () => {
  assert.equal(i18n.normalizeLocale("zh-Hant-TW"), "zh-TW");
  assert.equal(i18n.normalizeLocale("zh-HK"), "zh-TW");
  assert.equal(i18n.normalizeLocale("en-US"), "en");
  assert.equal(i18n.normalizeLocale("de-DE"), "de");
  assert.equal(i18n.normalizeLocale("fr-FR"), null);
});

test("translations interpolate values and switch at runtime", () => {
  i18n.setLocale("en", { notify: false });
  assert.equal(i18n.t("media.photos.results", { count: "1,234" }), "1,234 photos found");
  i18n.setLocale("de", { notify: false });
  assert.equal(i18n.t("common.remove", { label: "2026" }), "2026 entfernen");
  i18n.setLocale("zh-TW", { notify: false });
  assert.equal(i18n.t("folder.browse", { media: "照片" }), "瀏覽照片");
});

test("HTML translation attributes reference known keys", () => {
  const html = fs.readFileSync(`${__dirname}/static/index.html`, "utf8");
  const keys = [...html.matchAll(/data-i18n(?:-placeholder|-aria-label|-title|-alt)?="([^"]+)"/g)].map((match) => match[1]);
  for (const key of keys) assert.ok(i18n.translations.en[key], key);
});

test("literal app translation calls reference known keys", () => {
  const app = fs.readFileSync(`${__dirname}/static/app.js`, "utf8");
  const keys = [...app.matchAll(/\bt\("([^"]+)"/g)].map((match) => match[1]);
  for (const key of keys) assert.ok(i18n.translations.en[key], key);
});
