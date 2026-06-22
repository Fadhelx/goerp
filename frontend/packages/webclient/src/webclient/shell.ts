import type { ThemeTokens } from "../../../theme-tokens/src/index";
import {
  cleanAppName,
  homeMenuAppsCatalogApp,
  homeMenuEntry,
  normalizeHomeMenuApps,
  type HomeMenuApp,
  type HomeMenuPayload
} from "../home_menu/app_metadata.js";
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
  root.dataset.view = options.menus ? "apps" : "ready";
  root.dataset.mobileSafe = "true";
  const menuApps = normalizeHomeMenuApps(options.menus);
  const menuActions = normalizeHomeMenuApps(options.menus, { includeDescendantActions: true });
  const apps = options.apps ?? navbarApps(menuApps);
  const action = document.createElement("section");
  action.className = "o_action_manager";
  const setHomeMenuBackground = (active: boolean) => {
    toggleClassName(root, "o_home_menu_background", active);
    toggleClassName(document.body as HTMLElement | undefined, "o_home_menu_background", active);
  };
  const setMobileMenuOpen = (open: boolean) => {
    document.body?.classList?.toggle("o-mobile-menu-open", open);
  };
  let setNavbarActive: (appId?: number | string, brandName?: string) => void = () => {};
  let setNavbarApps: (apps: readonly NavbarApp[], activeAppId?: number | string, brandName?: string) => void = () => {};
  let activeBrandApp: HomeMenuApp | undefined;
  const openApp = (app: HomeMenuApp) => {
    root.dataset.view = "action";
    setHomeMenuBackground(false);
    setMobileMenuOpen(false);
    const appName = cleanAppName(app.name);
    const catalogApp = appName.toLowerCase() === "apps";
    const brandApp = catalogApp ? app : app.rootId === undefined ? app : menuApps.find((item) => String(item.id) === String(app.rootId)) ?? app;
    activeBrandApp = brandApp;
    const sections = catalogApp ? [{ id: app.id, name: appName }] : navbarSectionApps(options.menus, brandApp);
    const activeSectionID = catalogApp ? app.id : app.rootId === undefined ? app.id : navbarActiveSectionId(options.menus, brandApp, app) ?? app.rootId;
    setNavbarApps(sections.length ? sections : apps, activeSectionID, brandApp.name);
    return options.onOpenApp?.(app, action);
  };
  const openAppsCatalog = () => {
    const catalogApp = homeMenuAppsCatalogApp(options.menus);
    if (catalogApp) return openApp(catalogApp);
    return options.onOpenAppsCatalog?.(action);
  };
  const renderApps = () => {
    root.dataset.view = "apps";
    setHomeMenuBackground(true);
    setMobileMenuOpen(false);
    activeBrandApp = undefined;
    setNavbarApps(apps, undefined);
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
      const menuApp = menuActions.find((item) => String(item.id) === String(app.id))
        ?? firstSectionAction(menuActions, app.name, activeBrandApp);
      if (menuApp) openApp(menuApp);
    },
    onSystrayAction: (systrayAction) => {
      setMobileMenuOpen(false);
      options.onSystrayAction?.(systrayAction, action);
    }
  });
  setNavbarActive = navbar.setActiveApp;
  setNavbarApps = navbar.setApps;

  if (options.menus) {
    renderApps();
  } else {
    setHomeMenuBackground(false);
    const ready = document.createElement("section");
    ready.className = "o-control-panel o_control_panel";
    ready.textContent = options.debug ? "Debug" : "Ready";
    action.append(ready);
  }

  root.append(navbar, action);
  return root;
}

function toggleClassName(node: HTMLElement | undefined, className: string, active: boolean): void {
  if (!node) return;
  if (node.classList && typeof node.classList.toggle === "function") {
    node.classList.toggle(className, active);
    return;
  }
  const classes = new Set(String(node.className ?? "").split(/\s+/).filter(Boolean));
  if (active) {
    classes.add(className);
  } else {
    classes.delete(className);
  }
  node.className = [...classes].join(" ");
}

function navbarApps(apps: readonly HomeMenuApp[]): NavbarApp[] {
  return apps.map((app) => ({
    id: app.id,
    name: app.name
  }));
}

function navbarSectionApps(payload: HomeMenuPayload | undefined, app: HomeMenuApp): NavbarApp[] {
  if (!payload) return [];
  return (app.menu.children ?? [])
    .map((id) => homeMenuEntry(payload, id))
    .filter((entry): entry is NonNullable<ReturnType<typeof homeMenuEntry>> => Boolean(entry))
    .map((entry) => ({
      id: entry.id ?? cleanAppName(entry.name),
      name: cleanAppName(entry.name)
    }));
}

function firstSectionAction(actions: readonly HomeMenuApp[], sectionName: string, activeRoot: HomeMenuApp | undefined): HomeMenuApp | undefined {
  if (!activeRoot) return undefined;
  const prefix = `${activeRoot.name} / ${sectionName}`;
  return actions.find((item) => item.parentPath === prefix || item.parentPath?.startsWith(`${prefix} / `));
}

function navbarActiveSectionId(payload: HomeMenuPayload | undefined, brandApp: HomeMenuApp, app: HomeMenuApp): number | string | undefined {
  if (!payload) return undefined;
  const parentPath = app.parentPath ?? "";
  for (const sectionId of brandApp.menu.children ?? []) {
    const section = homeMenuEntry(payload, sectionId);
    if (!section) continue;
    const sectionName = cleanAppName(section.name);
    const prefix = `${brandApp.name} / ${sectionName}`;
    if (String(app.id) === String(section.id ?? sectionId) || parentPath === prefix || parentPath.startsWith(`${prefix} / `)) {
      return section.id ?? sectionId;
    }
  }
  return undefined;
}
