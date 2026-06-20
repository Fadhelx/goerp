export interface ThemeTokens {
  name: string;
  color: Record<string, string>;
  typography: Record<string, string>;
  radius: Record<string, string>;
  spacing: Record<string, string>;
  density: "compact" | "comfortable";
}

export function toCSSVariables(tokens: ThemeTokens): string {
  const lines: string[] = [];
  for (const [group, values] of Object.entries({
    color: tokens.color,
    typography: tokens.typography,
    radius: tokens.radius,
    spacing: tokens.spacing
  })) {
    for (const [key, value] of Object.entries(values)) {
      lines.push(`--gorp-${group}-${key}: ${value};`);
    }
  }
  lines.push(`--gorp-density: ${tokens.density};`);
  return `:root {\n  ${lines.join("\n  ")}\n}`;
}
