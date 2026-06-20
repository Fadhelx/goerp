import type { ThemeTokens } from "../../../packages/theme-tokens/src/index";

export const standardTheme: ThemeTokens = {
  name: "standard",
  color: {
    surface: "#ffffff",
    panel: "#f5f7f9",
    text: "#17212b",
    accent: "#2563eb",
    focus: "#0f766e"
  },
  typography: {
    body: "ui-sans-serif, system-ui",
    mono: "ui-monospace, SFMono-Regular, Menlo"
  },
  radius: {
    control: "4px",
    panel: "6px"
  },
  spacing: {
    xs: "4px",
    sm: "8px",
    md: "12px",
    lg: "16px"
  },
  density: "comfortable"
};
