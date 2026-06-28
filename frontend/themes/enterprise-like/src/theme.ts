import type { ThemeTokens } from "../../../packages/theme-tokens/src/index";

export const enterpriseLikeTheme: ThemeTokens = {
  name: "enterprise-like",
  color: {
    surface: "#f5f5f5",
    panel: "#ffffff",
    text: "#1f2933",
    accent: "#875a7b",
    focus: "#00a09d",
    navbar: "#262a36",
    home: "#000511",
    homeText: "#ffffff"
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
  density: "compact"
};
