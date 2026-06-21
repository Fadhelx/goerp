import type { ThemeTokens } from "../../../theme-tokens/src/index";
import { normalizeHomeMenuApps, type HomeMenuApp, type HomeMenuPayload } from "../home_menu/app_metadata.js";
import { renderHomeMenu } from "../home_menu/home_menu.js";
import { renderNavbar, type NavbarApp } from "./navbar/navbar.js";

export interface WebClientShellOptions {
  theme: ThemeTokens;
  debug?: boolean;
  apps?: readonly NavbarApp[];
  userName?: string;
  companyName?: string;
  menus?: HomeMenuPayload;
  onOpenApp?: (app: HomeMenuApp, outlet: HTMLElement) => unknown;
  onOpenAppsCatalog?: (outlet: HTMLElement) => unknown;
}

export function createWebClientShell(options: WebClientShellOptions): HTMLElement {
  const root = document.createElement("main");
  root.className = "o_web_client";
  root.dataset.theme = options.theme.name;
  root.dataset.mobileSafe = "true";
  const menuApps = normalizeHomeMenuApps(options.menus);
  const apps = options.apps ?? navbarApps(menuApps);
  const action = document.createElement("section");
  action.className = "o_action_manager";
  const setMobileMenuOpen = (open: boolean) => {
    document.body?.classList?.toggle("o-mobile-menu-open", open);
  };
  const openApp = (app: HomeMenuApp) => {
    setMobileMenuOpen(false);
    return options.onOpenApp?.(app, action);
  };
  const renderApps = () => {
    setMobileMenuOpen(false);
    if (!options.menus) return;
    action.replaceChildren(renderHomeMenu(options.menus, {
      onOpenApp: openApp,
      onOpenAppsCatalog: () => options.onOpenAppsCatalog?.(action)
    }));
  };

  const navbar = renderNavbar({
    apps,
    userName: options.userName,
    companyName: options.companyName,
    debug: options.debug,
    onOpenApps: renderApps,
    onToggleMobileMenu: setMobileMenuOpen,
    onOpenApp: (app) => {
      const menuApp = menuApps.find((item) => String(item.id) === String(app.id));
      if (menuApp) openApp(menuApp);
    }
  });

  if (options.menus) {
    renderApps();
  } else {
    const ready = document.createElement("section");
    ready.className = "o-control-panel o_control_panel";
    ready.textContent = options.debug ? "Debug" : "Ready";
    action.append(ready);
  }

  root.append(navbar, action);
  return root;
}

function navbarApps(apps: readonly HomeMenuApp[]): NavbarApp[] {
  return apps.map((app) => ({
    id: app.id,
    name: app.name
  }));
}
