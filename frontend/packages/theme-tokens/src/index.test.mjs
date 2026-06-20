import assert from "node:assert/strict";
import { toCSSVariables } from "../../../dist/packages/theme-tokens/src/index.js";

const css = toCSSVariables({
  name: "standard",
  color: { surface: "#ffffff" },
  typography: { body: "system-ui" },
  radius: { control: "4px" },
  spacing: { sm: "8px" },
  density: "comfortable"
});

assert.match(css, /--gorp-color-surface/);
