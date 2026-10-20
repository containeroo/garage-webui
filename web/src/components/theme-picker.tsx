import { useEffect, useRef, useState } from "react";
import { errorMessage, request } from "../api";

/** A named color scheme supplied by the backend theme catalog. */
type Theme = {
  /** Human-readable theme or dialog title. */
  title: string;
  /** Browser color scheme used by the theme. */
  color_scheme: "light" | "dark";
  /** Theme color tokens mapped to CSS custom properties. */
  colors: Record<string, string>;
};
/** Themes indexed by their stable display title. */
type Catalog = Record<string, Theme>;
/** applyTheme replaces document colors with those of the selected theme. */
function applyTheme(theme: Theme) {
  resetTheme();
  const root = document.documentElement;
  root.dataset.theme = theme.color_scheme;
  root.style.colorScheme = theme.color_scheme;
  for (const [key, value] of Object.entries(theme.colors))
    root.style.setProperty(`--theme-${key.replaceAll("_", "-")}`, value);
}
/** resetTheme removes session-specific colors from the document root. */
function resetTheme() {
  const root = document.documentElement;
  for (const key of Array.from(root.style))
    if (key.startsWith("--theme-")) root.style.removeProperty(key);
  root.style.removeProperty("color-scheme");
  delete root.dataset.theme;
}
/** useTheme loads, applies, and persists the authenticated session's appearance. */
export function useTheme() {
  const [catalog, setCatalog] = useState<Catalog>({});
  const [selected, setSelected] = useState("Light");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const saving = useRef<AbortController | null>(null);
  useEffect(() => {
    let active = true;
    Promise.all([
      request<Theme[]>("api/themes"),
      request<{
        /** Session appearance preference and its update controls. */
        theme: string;
      }>("api/profile"),
    ])
      .then(([files, profile]) => {
        if (!active) return;
        const themes = Object.fromEntries(
          files.map((theme) => [theme.title, theme]),
        );
        if (!themes.Light) throw new Error("default theme unavailable");
        const name = themes[profile.theme] ? profile.theme : "Light";
        setCatalog(themes);
        setSelected(name);
        applyTheme(themes[name]);
      })
      .catch((cause) => {
        if (active) setError(errorMessage(cause));
      });
    return () => {
      active = false;
      saving.current?.abort();
      resetTheme();
    };
  }, []);
  /** change saves the selected theme and discards superseded or unmounted requests. */
  async function change(name: string) {
    if (!catalog[name]) return;
    saving.current?.abort();
    const controller = new AbortController();
    saving.current = controller;
    setBusy(true);
    setError("");
    try {
      await request("api/profile/theme", {
        method: "PUT",
        body: JSON.stringify({ theme: name }),
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      setSelected(name);
      applyTheme(catalog[name]);
    } catch (cause) {
      if (!controller.signal.aborted) setError(errorMessage(cause));
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return {
    selected,
    names: Object.keys(catalog).sort((a, b) => a.localeCompare(b)),
    busy,
    error,
    change,
  };
}
/** The current session theme and its persistence controls. */
export type ThemePreference = ReturnType<typeof useTheme>;
