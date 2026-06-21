import type { ThemeTokens } from "../../../theme-tokens/src/index";
import { normalizeHomeMenuApps, type HomeMenuPayload } from "../home_menu/app_metadata.js";
import { renderHomeMenu } from "../home_menu/home_menu.js";
import { renderNavbar, type NavbarApp } from "./navbar/navbar.js";

export interface WebClientShellOptions {
  theme: ThemeTokens;
  debug?: boolean;
  apps?: readonly NavbarApp[];
  userName?: string;
  companyName?: string;
  menus?: HomeMenuPayload;
}

export function createWebClientShell(options: WebClientShellOptions): HTMLElement {
  const root = document.createElement("main");
  root.className = "o_web_client";
  root.dataset.theme = options.theme.name;
  root.dataset.mobileSafe = "true";
  const apps = options.apps ?? navbarAppsFromMenus(options.menus);

  const navbar = renderNavbar({
    apps,
    userName: options.userName,
    companyName: options.companyName,
    debug: options.debug
  });

  const action = document.createElement("section");
  action.className = "o_action_manager";
  if (options.menus) {
    action.append(renderHomeMenu(options.menus));
  } else {
    const ready = document.createElement("section");
    ready.className = "o-control-panel o_control_panel";
    ready.textContent = options.debug ? "Debug" : "Ready";
    action.append(ready);
  }

  root.append(navbar, action);
  return root;
}

function navbarAppsFromMenus(menus: HomeMenuPayload | undefined): NavbarApp[] {
  return normalizeHomeMenuApps(menus).map((app) => ({
    id: app.id,
    name: app.name
  }));
}
