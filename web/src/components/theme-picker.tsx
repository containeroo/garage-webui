import { useEffect, useState } from "react";
import { errorMessage, request } from "../api";

type Theme = {
  title: string;
  color_scheme: "light" | "dark";
  colors: Record<string, string>;
};
type Catalog = Record<string, Theme>;
function applyTheme(theme: Theme) {
  const root = document.documentElement;
  root.dataset.theme = theme.color_scheme;
  root.style.colorScheme = theme.color_scheme;
  for (const [key, value] of Object.entries(theme.colors))
    root.style.setProperty(`--theme-${key.replaceAll("_", "-")}`, value);
}
function resetTheme() {
  const root = document.documentElement;
  for (const key of Array.from(root.style))
    if (key.startsWith("--theme-")) root.style.removeProperty(key);
  root.style.removeProperty("color-scheme");
  delete root.dataset.theme;
}
export function useTheme() {
  const [catalog, setCatalog] = useState<Catalog>({});
  const [selected, setSelected] = useState("Light");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    Promise.all([
      request<Theme[]>("api/themes"),
      request<{ theme: string }>("api/profile"),
    ])
      .then(([files, profile]) => {
        if (!active) return;
        const themes = Object.fromEntries(
          files.map((theme) => [theme.title, theme]),
        );
        if (!themes.Light) throw new Error("Default theme unavailable");
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
      resetTheme();
    };
  }, []);
  async function change(name: string) {
    if (!catalog[name]) return;
    setBusy(true);
    setError("");
    try {
      await request("api/profile/theme", {
        method: "PUT",
        body: JSON.stringify({ theme: name }),
      });
      setSelected(name);
      applyTheme(catalog[name]);
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
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
export type ThemePreference = ReturnType<typeof useTheme>;
