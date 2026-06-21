import type { ThemeTokens } from "../../../theme-tokens/src/index";
import { homeMenuAppsCatalogApp, normalizeHomeMenuApps, type HomeMenuApp, type HomeMenuPayload } from "../home_menu/app_metadata.js";
import { renderHomeMenu } from "../home_menu/home_menu.js";
import { renderNavbar, type NavbarApp, type NavbarSystrayAction, type NavbarSystrayState, type RenderedNavbar } from "./navbar/navbar.js";

export interface WebClientShellOptions {
  theme: ThemeTokens;
  debug?: boolean;
  apps?: readonly NavbarApp[];
  userName?: string;
  companyName?: string;
  systray?: NavbarSystrayState;
  menus?: HomeMenuPayload;
  onOpenApp?: (app: HomeMenuApp, outlet: HTMLElement) => unknown;
  onOpenAppsCatalog?: (outlet: HTMLElement) => unknown;
  onSystrayAction?: (action: NavbarSystrayAction, outlet: HTMLElement) => unknown;
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
  let setNavbarActive: (appId?: number | string) => void = () => {};
  const openApp = (app: HomeMenuApp) => {
    setMobileMenuOpen(false);
    setNavbarActive(app.rootId ?? app.id);
    return options.onOpenApp?.(app, action);
  };
  const openAppsCatalog = () => {
    const catalogApp = homeMenuAppsCatalogApp(options.menus);
    if (catalogApp) return openApp(catalogApp);
    return options.onOpenAppsCatalog?.(action);
  };
  const renderApps = () => {
    setMobileMenuOpen(false);
    setNavbarActive(undefined);
    if (!options.menus) return;
    action.replaceChildren(renderHomeMenu(options.menus, {
      onOpenApp: openApp,
      onOpenAppsCatalog: openAppsCatalog
    }));
  };

  const navbar: RenderedNavbar = renderNavbar({
    apps,
    userName: options.userName,
    companyName: options.companyName,
    debug: options.debug,
    systray: options.systray,
    onOpenApps: renderApps,
    onToggleMobileMenu: setMobileMenuOpen,
    onOpenApp: (app) => {
      const menuApp = menuApps.find((item) => String(item.id) === String(app.id));
      if (menuApp) openApp(menuApp);
    },
    onSystrayAction: (systrayAction) => {
      setMobileMenuOpen(false);
      options.onSystrayAction?.(systrayAction, action);
    }
  });
  setNavbarActive = navbar.setActiveApp;

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
