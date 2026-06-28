import type { ThemeTokens } from "../../../theme-tokens/src/index";
import {
  cleanAppName,
  homeMenuAppsCatalogApp,
  homeMenuEntry,
  menuActionValue,
  menuDirectActionValue,
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
  databaseName?: string;
  systray?: NavbarSystrayState;
  menus?: HomeMenuPayload;
  onOpenApp?: (app: HomeMenuApp, outlet: HTMLElement) => unknown;
  onOpenAppsCatalog?: (outlet: HTMLElement) => unknown;
  onSystrayAction?: (action: NavbarSystrayAction, outlet: HTMLElement) => unknown;
}

export interface RenderedWebClientShell extends HTMLElement {
  openMenuApp: (menuId: number | string) => unknown;
  setMenuContext: (menuId: number | string) => boolean;
}

export function createWebClientShell(options: WebClientShellOptions): RenderedWebClientShell {
  const root = document.createElement("main") as RenderedWebClientShell;
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
  const setShellView = (view: "apps" | "action" | "ready", homeMenuMode?: "root" | "overlay") => {
    root.dataset.view = view;
    syncBodyShellView(view, homeMenuMode);
    if (homeMenuMode) {
      root.dataset.homeMenuMode = homeMenuMode;
      return;
    }
    delete root.dataset.homeMenuMode;
  };
  const setMobileMenuOpen = (open: boolean) => {
    document.body?.classList?.toggle("o-mobile-menu-open", open);
  };
  let setNavbarActive: (appId?: number | string, brandName?: string) => void = () => {};
  let setNavbarApps: (apps: readonly NavbarApp[], activeAppId?: number | string, brandName?: string) => void = () => {};
  let setNavbarHomeMenuBackMode: (enabled: boolean) => void = () => {};
  let activeBrandApp: HomeMenuApp | undefined;
  let previousActionChildren: HTMLElement[] = [];
  const applyMenuContext = (app: HomeMenuApp) => {
    previousActionChildren = [];
    setShellView("action");
    setNavbarHomeMenuBackMode(false);
    setHomeMenuBackground(false);
    setMobileMenuOpen(false);
    const appName = cleanAppName(app.name);
    const catalogApp = appName.toLowerCase() === "apps";
    const brandApp = catalogApp ? app : app.rootId === undefined ? app : menuApps.find((item) => String(item.id) === String(app.rootId)) ?? app;
    activeBrandApp = brandApp;
    const sections = catalogApp ? appsCatalogNavbarSections(app) : navbarSectionApps(options.menus, brandApp);
    const activeSectionID = catalogApp ? app.id : app.rootId === undefined ? app.id : navbarActiveSectionId(options.menus, brandApp, app) ?? app.rootId;
    setNavbarApps(sections.length ? sections : apps, activeSectionID, brandApp.name);
  };
  const openApp = (app: HomeMenuApp) => {
    applyMenuContext(app);
    return options.onOpenApp?.(app, action);
  };
  const openAppsCatalog = () => {
    const catalogApp = homeMenuAppsCatalogApp(options.menus);
    if (catalogApp) return openApp(catalogApp);
    return options.onOpenAppsCatalog?.(action);
  };
  const renderRootApps = () => {
    previousActionChildren = [];
    setShellView("apps", "root");
    setNavbarHomeMenuBackMode(false);
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
  const renderOverlayApps = () => {
    if (root.dataset.view !== "apps") {
      previousActionChildren = Array.from(action.children) as HTMLElement[];
    }
    setShellView("apps", "overlay");
    setNavbarHomeMenuBackMode(true);
    setHomeMenuBackground(true);
    setMobileMenuOpen(false);
    if (!options.menus) return;
    action.replaceChildren(renderHomeMenu(options.menus, {
      onOpenApp: openApp,
      onOpenAppsCatalog: openAppsCatalog
    }));
  };
  const restoreActionFromOverlayApps = () => {
    setShellView("action");
    setNavbarHomeMenuBackMode(false);
    setHomeMenuBackground(false);
    setMobileMenuOpen(false);
    action.replaceChildren(...previousActionChildren);
    previousActionChildren = [];
  };
  const toggleAppsMenu = () => {
    if (root.dataset.view === "apps" && root.dataset.homeMenuMode === "overlay") {
      restoreActionFromOverlayApps();
      return;
    }
    if (root.dataset.view === "action") {
      renderOverlayApps();
      return;
    }
    renderRootApps();
  };

  const navbar: RenderedNavbar = renderNavbar({
    apps,
    userName: options.userName,
    companyName: options.companyName,
    databaseName: options.databaseName,
    debug: options.debug,
    systray: options.systray,
    onOpenApps: toggleAppsMenu,
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
  setNavbarHomeMenuBackMode = navbar.setHomeMenuBackMode;
  root.openMenuApp = (menuId: number | string) => {
    const menuApp = menuActions.find((item) => String(item.id) === String(menuId));
    if (!menuApp) return undefined;
    return openApp(menuApp);
  };
  root.setMenuContext = (menuId: number | string): boolean => {
    const menuApp = menuActions.find((item) => String(item.id) === String(menuId));
    if (!menuApp) return false;
    applyMenuContext(menuApp);
    return true;
  };

  if (options.menus) {
    renderRootApps();
  } else {
    setShellView("ready");
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

function syncBodyShellView(view: "apps" | "action" | "ready", homeMenuMode?: "root" | "overlay"): void {
  const body = document.body as HTMLElement | undefined;
  if (!body?.dataset) return;
  body.dataset.view = view;
  if (homeMenuMode) {
    body.dataset.homeMenuMode = homeMenuMode;
  } else {
    delete body.dataset.homeMenuMode;
  }
}

function navbarApps(apps: readonly HomeMenuApp[]): NavbarApp[] {
  return apps.map((app) => ({
    id: app.id,
    name: app.name
  }));
}

function navbarSectionApps(payload: HomeMenuPayload | undefined, app: HomeMenuApp): NavbarApp[] {
  if (!payload) return [];
  const sections = (app.menu.children ?? [])
    .map((id) => navbarMenuEntry(payload, id))
    .filter((entry): entry is NavbarApp => Boolean(entry));
  return normalizeSettingsNavbarSections(app, sections);
}

function navbarMenuEntry(payload: HomeMenuPayload, id: number | string): NavbarApp | null {
  const entry = homeMenuEntry(payload, id);
  if (!entry) return null;
  const children = (entry.children ?? [])
    .map((childId) => navbarMenuEntry(payload, childId))
    .filter((child): child is NavbarApp => Boolean(child));
  return {
    id: entry.id ?? id,
    name: cleanAppName(entry.name),
    action: menuDirectActionValue(entry) !== undefined || (menuActionValue(entry) !== undefined && !children.length),
    children
  };
}

function appsCatalogNavbarSections(app: HomeMenuApp): NavbarApp[] {
  return [
    { id: app.id, name: "Apps", action: true }
  ];
}

function normalizeSettingsNavbarSections(app: HomeMenuApp, sections: readonly NavbarApp[]): NavbarApp[] {
  if (cleanAppName(app.name).toLowerCase() !== "settings") return [...sections];
  return sections.map((section) => cleanAppName(section.name) === "Technical" ? normalizeTechnicalNavbarSection(section) : section);
}

const TECHNICAL_GROUP_ORDER = [
  "Email",
  "Actions",
  "IAP",
  "User Interface",
  "Database Structure",
  "Automation",
  "Reporting",
  "Sequences & Identifiers",
  "Parameters",
  "Security"
];

const TECHNICAL_CHILD_ORDER: Record<string, readonly string[]> = {
  Email: ["Outgoing Mail Servers"],
  Actions: [
    "Actions",
    "Reports",
    "Window Actions",
    "Client Actions",
    "Server Actions",
    "Embedded Actions",
    "Configuration Wizards",
    "User-defined Defaults"
  ],
  IAP: ["IAP Accounts"],
  "User Interface": ["Menu Items", "Views", "Customized Views", "User-defined Filters", "Tours"],
  "Database Structure": [
    "Decimal Accuracy",
    "Assets",
    "Models",
    "Fields",
    "Fields Selection",
    "Model Constraints",
    "ManyToMany Relations",
    "Attachments",
    "Logging",
    "Profiling"
  ],
  Automation: ["Scheduled Actions", "Scheduled Actions Triggers"],
  Reporting: ["Paper Format", "Reports"],
  "Sequences & Identifiers": ["External Identifiers", "Sequences"],
  Parameters: ["System Parameters"],
  Security: ["Record Rules", "Access Rights", "User Devices"]
};

const TECHNICAL_REFERENCE_PLACEHOLDERS: Record<string, Record<string, NavbarApp>> = {
  Actions: {
    Actions: technicalPlaceholder("Actions", "actions"),
    Reports: technicalPlaceholder("Reports", "reports"),
    "Window Actions": technicalPlaceholder("Window Actions", "window_actions"),
    "Client Actions": technicalPlaceholder("Client Actions", "client_actions"),
    "Server Actions": technicalPlaceholder("Server Actions", "server_actions"),
    "Embedded Actions": technicalPlaceholder("Embedded Actions", "embedded_actions"),
    "Configuration Wizards": technicalPlaceholder("Configuration Wizards", "configuration_wizards"),
    "User-defined Defaults": technicalPlaceholder("User-defined Defaults", "user_defined_defaults")
  },
  IAP: {
    "IAP Accounts": technicalPlaceholder("IAP Accounts", "iap_accounts")
  },
  "User Interface": {
    Tours: technicalPlaceholder("Tours", "tours")
  },
  "Database Structure": {
    "Decimal Accuracy": technicalPlaceholder("Decimal Accuracy", "decimal_accuracy"),
    Assets: technicalPlaceholder("Assets", "assets"),
    "Fields Selection": technicalPlaceholder("Fields Selection", "fields_selection"),
    "Model Constraints": technicalPlaceholder("Model Constraints", "model_constraints"),
    "ManyToMany Relations": technicalPlaceholder("ManyToMany Relations", "many_to_many_relations"),
    Attachments: technicalPlaceholder("Attachments", "attachments"),
    Logging: technicalPlaceholder("Logging", "logging"),
    Profiling: technicalPlaceholder("Profiling", "profiling")
  },
  Reporting: {
    "Paper Format": technicalPlaceholder("Paper Format", "paper_format"),
    Reports: technicalPlaceholder("Reports", "reporting_reports")
  },
  Automation: {
    "Scheduled Actions": technicalPlaceholder("Scheduled Actions", "scheduled_actions"),
    "Scheduled Actions Triggers": technicalPlaceholder("Scheduled Actions Triggers", "scheduled_actions_triggers")
  },
  "Sequences & Identifiers": {
    "External Identifiers": technicalPlaceholder("External Identifiers", "external_identifiers"),
    Sequences: technicalPlaceholder("Sequences", "sequences")
  },
  Parameters: {
    "System Parameters": technicalPlaceholder("System Parameters", "system_parameters")
  },
  Security: {
    "Record Rules": technicalPlaceholder("Record Rules", "record_rules"),
    "Access Rights": technicalPlaceholder("Access Rights", "access_rights"),
    "User Devices": technicalPlaceholder("User Devices", "user_devices")
  }
};

function normalizeTechnicalNavbarSection(section: NavbarApp): NavbarApp {
  const children = (section.children ?? []).map(normalizeTechnicalNavbarLabels);
  const byName = new Map(children.map((child) => [child.name, child]));
  const ordered: NavbarApp[] = [];
  const used = new Set<string>();
  for (const groupName of TECHNICAL_GROUP_ORDER) {
    const existing = byName.get(groupName);
    if (!existing && !TECHNICAL_REFERENCE_PLACEHOLDERS[groupName]) continue;
    const group = existing ?? { id: `technical:${technicalKey(groupName)}`, name: groupName, action: false, children: [] };
    used.add(group.name);
    ordered.push(normalizeTechnicalNavbarGroup(group));
  }
  return { ...section, children: ordered };
}

function normalizeTechnicalNavbarGroup(group: NavbarApp): NavbarApp {
  const children = group.children ?? [];
  const order = TECHNICAL_CHILD_ORDER[group.name];
  if (!order) return { ...group, children };
  const byName = new Map(children.map((child) => [child.name, child]));
  const placeholders = TECHNICAL_REFERENCE_PLACEHOLDERS[group.name] ?? {};
  const ordered: NavbarApp[] = [];
  const used = new Set<string>();
  for (const name of order) {
    const child = byName.get(name) ?? placeholders[name];
    if (!child) continue;
    ordered.push(child);
    used.add(name);
  }
  return { ...group, children: ordered };
}

function normalizeTechnicalNavbarLabels(entry: NavbarApp): NavbarApp {
  const name = technicalNavbarLabel(entry.name);
  return {
    ...entry,
    name,
    children: entry.children?.map(normalizeTechnicalNavbarLabels)
  };
}

function technicalNavbarLabel(name: string): string {
  switch (cleanAppName(name)) {
    case "Report":
    case "Report Actions":
    case "Reports":
      return "Reports";
    case "Outgoing Mail Server":
    case "Outgoing Mail Servers":
      return "Outgoing Mail Servers";
    case "Incoming Mail Servers":
      return "Incoming Mail Server";
    case "Window Action":
    case "Window Actions":
      return "Window Actions";
    case "Client Action":
    case "Client Actions":
      return "Client Actions";
    case "Server Action":
    case "Server Actions":
      return "Server Actions";
    case "Selection Values":
      return "Fields Selection";
    case "Many-to-Many Relations":
      return "ManyToMany Relations";
    case "Paper Formats":
      return "Paper Format";
    case "Scheduled Action Triggers":
      return "Scheduled Actions Triggers";
    default:
      return cleanAppName(name);
  }
}

function technicalPlaceholder(name: string, key: string): NavbarApp {
  return { id: `technical:${key}`, name, action: true };
}

function technicalKey(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
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
