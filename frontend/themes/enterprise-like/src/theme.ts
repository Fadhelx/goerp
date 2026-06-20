import type { ThemeTokens } from "../../../packages/theme-tokens/src/index";

export const enterpriseLikeTheme: ThemeTokens = {
  name: "enterprise-like",
  color: {
    surface: "#f4f5f7",
    panel: "#ffffff",
    text: "#1f2933",
    accent: "#714b67",
    focus: "#017e84",
    navbar: "#714b67",
    home: "#241723",
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
