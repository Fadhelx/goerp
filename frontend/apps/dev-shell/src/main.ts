import { createWebClient } from "../../../packages/webclient/src/index.js";
import { standardTheme } from "../../../themes/standard/src/theme.js";

export function mountDevShell(target: HTMLElement) {
  const app = createWebClient({
    env: { debug: true, isSmall: false },
    theme: standardTheme
  });
  target.replaceChildren(app.render());
}
