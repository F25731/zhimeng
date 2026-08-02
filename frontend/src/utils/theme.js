const storageKey = "control-center-theme";

export function getTheme() {
  try {
    return localStorage.getItem(storageKey) === "dark" ? "dark" : "light";
  } catch (_) {
    return "light";
  }
}

export function applyTheme(theme) {
  const value = theme === "dark" ? "dark" : "light";
  document.documentElement.dataset.theme = value;
  try { localStorage.setItem(storageKey, value); } catch (_) { /* Storage may be unavailable. */ }
  window.dispatchEvent(new CustomEvent("control-theme-change", { detail: value }));
  return value;
}
