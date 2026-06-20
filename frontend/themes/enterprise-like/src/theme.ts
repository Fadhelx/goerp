import type { ThemeTokens } from "../../../packages/theme-tokens/src/index";

export const enterpriseLikeTheme: ThemeTokens = {
  name: "enterprise-like",
  color: {
    surface: "#f7f8fa",
    panel: "#ffffff",
    text: "#1d2733",
    accent: "#0f766e",
    focus: "#2563eb"
  },
  typography: {
    body: "Inter, ui-sans-serif, system-ui",
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
  density: "compact"
};
