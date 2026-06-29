import {
  appKey,
  homeMenuAppsCatalogApp,
  isAppsCatalogApp,
  normalizeHomeMenuApps,
  type HomeMenuApp,
  type HomeMenuPayload
} from "./app_metadata.js";

export interface HomeMenuRenderOptions {
  query?: string;
  includeAppsCatalog?: boolean;
  onOpenApp?: (app: HomeMenuApp) => void;
  onOpenAppsCatalog?: (app?: HomeMenuApp) => void;
}

export function renderHomeMenu(payload: HomeMenuPayload, options: HomeMenuRenderOptions = {}): HTMLElement {
  const section = document.createElement("section");
  section.className = "o-app-launcher-view o_app_launcher o_home_menu_background";
  section.dataset.view = "apps";
  section.dataset.mobileSafe = "true";
  section.setAttribute("tabindex", "-1");

  const shell = document.createElement("div");
  shell.className = "o-app-shell o_home_menu h-100 overflow-auto";

  const container = document.createElement("div");
  container.className = "container o_home_menu_container";

  const registrationBanner = document.createElement("div");
  registrationBanner.className = "o_home_menu_registration_banner";
  registrationBanner.setAttribute("role", "status");
  const registrationText = document.createElement("span");
  registrationText.className = "o_home_menu_registration_text";
  registrationText.textContent = "You will be able to register your database once you have installed your first app.";
  const registrationClose = document.createElement("button");
  registrationClose.type = "button";
  registrationClose.className = "o_home_menu_registration_close";
  registrationClose.setAttribute("aria-label", "Dismiss");
  registrationClose.textContent = "Dismiss";
  registrationClose.addEventListener("click", () => {
    registrationBanner.hidden = true;
    registrationBanner.dataset.dismissed = "true";
  });
  registrationBanner.append(registrationText, registrationClose);

  const searchWrap = document.createElement("div");
  searchWrap.className = "o-app-search o_home_menu_search";
  const search = document.createElement("input");
  search.type = "text";
  search.className = "o_app_search_stub o_search_hidden visually-hidden";
  search.setAttribute("data-allow-hotkeys", "true");
  search.setAttribute("aria-label", "Search apps and menus");
  search.setAttribute("role", "combobox");
  search.setAttribute("aria-autocomplete", "list");
  search.setAttribute("aria-haspopup", "listbox");
  search.value = options.query ?? "";
  searchWrap.append(search);

  const grid = document.createElement("div");
  grid.className = "o_apps row user-select-none mt-3 mx-0";
  grid.setAttribute("role", "listbox");

  const setSearchActive = (active: boolean) => {
    searchWrap.className = active ? "o-app-search o_home_menu_search is-active" : "o-app-search o_home_menu_search";
    search.className = active ? "o_app_search_input" : "o_app_search_stub o_search_hidden visually-hidden";
    searchWrap.dataset.searchActive = active ? "true" : "false";
    search.setAttribute("aria-expanded", active && grid.children.length ? "true" : "false");
  };
  const renderGrid = () => {
    const query = search.value.trim().toLowerCase();
    setSearchActive(Boolean(query));
    const apps = normalizeHomeMenuApps(payload, { includeDescendantActions: Boolean(query) });
    const catalogApp = homeMenuAppsCatalogApp(payload);
    const visible = query ? apps.filter((app) => app.searchText.includes(query)) : launcherRootApps(apps, catalogApp);
    grid.replaceChildren();
    for (const app of visible) {
      grid.append(renderHomeMenuApp(app, () => options.onOpenApp?.(app)));
    }

    if (options.includeAppsCatalog !== false && shouldAppendAppsCatalog(query, visible, catalogApp)) {
      const app: HomeMenuApp = catalogApp ?? {
        id: "apps",
        key: "apps",
        name: "Apps",
        initials: "A",
        iconToken: "teal",
        sequence: apps.length,
        searchText: APPS_CATALOG_SEARCH_TEXT,
        menu: { id: "apps", name: "Apps" }
      };
      const launcherApp = query ? app : { ...app, key: "apps", initials: "A", iconToken: "apps", parentPath: undefined };
      grid.append(renderHomeMenuApp(launcherApp, () => {
        if (catalogApp && options.onOpenApp) {
          options.onOpenApp?.(catalogApp);
        } else {
          options.onOpenAppsCatalog?.(catalogApp ?? undefined);
        }
      }));
    }

    if (!grid.children.length) {
      const empty = document.createElement("p");
      empty.className = "muted";
      empty.textContent = query ? "No apps found." : "No menus loaded.";
      grid.append(empty);
    }
    search.setAttribute("aria-expanded", query && grid.querySelector?.(".o_app") ? "true" : "false");
  };
  const showSearch = () => setSearchActive(true);
  const hideSearchIfEmpty = () => {
    if (!search.value.trim()) setSearchActive(false);
  };
  search.addEventListener("focus", showSearch);
  search.addEventListener("blur", hideSearchIfEmpty);
  search.addEventListener("input", renderGrid);
  search.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    search.value = "";
    renderGrid();
    setSearchActive(false);
  });
  section.addEventListener("keydown", (event) => {
    if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return;
    if (isTextInput(event.target)) return;
    if (event.key === "/") {
      event.preventDefault?.();
      setSearchActive(true);
      search.focus?.();
      return;
    }
    if (event.key.length !== 1) return;
    event.preventDefault?.();
    search.value = event.key;
    setSearchActive(true);
    search.focus?.();
    renderGrid();
  });
  renderGrid();

  container.append(registrationBanner, searchWrap, grid);
  shell.append(container);
  section.append(homeMenuParityStyleElement(), shell);
  return section;
}

function homeMenuParityStyleElement(): HTMLElement {
  const style = document.createElement("style");
  style.dataset.homeMenuParity = "odoo19-banner";
  style.textContent = `
    .o_app_launcher .o_home_menu_registration_banner {
      box-sizing: border-box;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 12px;
      max-width: min(920px, calc(100vw - 32px));
      margin: 12px auto 0;
      padding: 7px 12px;
      overflow: hidden;
      border-radius: 3px;
    }
    .o_app_launcher .o_home_menu_registration_text {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .o_app_launcher .o_home_menu_registration_close {
      flex: 0 0 auto;
      max-width: 112px;
      min-height: 26px;
      padding: 2px 8px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    @media (max-width: 575px) {
      .o_app_launcher .o_home_menu_registration_banner {
        justify-content: flex-start;
        max-width: calc(100vw - 16px);
        margin: 8px 8px 0;
        padding: 6px 8px;
      }
      .o_app_launcher .o_home_menu_registration_close {
        max-width: 76px;
      }
    }
  `;
  return style;
}

function isTextInput(target: EventTarget | null): boolean {
  const tag = (target as HTMLElement | null)?.tagName?.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select";
}

export function renderHomeMenuApp(app: HomeMenuApp, onClick?: () => void): HTMLElement {
  const wrapper = document.createElement("div");
  wrapper.className = "col-3 col-md-2 o_draggable mb-3 px-0";

  const button = document.createElement("a");
  button.className = "o_app o_menuitem has-icon d-flex flex-column rounded-3 justify-content-start align-items-center w-100 p-1 p-md-2";
  button.setAttribute("role", "option");
  button.setAttribute("aria-selected", "false");
  button.setAttribute("href", appHref(app));
  button.dataset.appName = app.name;
  button.dataset.appKey = app.key || appKey(app.name);
  button.dataset.iconKind = launcherIconKind(app);
  button.dataset.menuId = String(app.id);
  if (typeof app.menu.xmlid === "string" && app.menu.xmlid) button.dataset.menuXmlid = app.menu.xmlid;
  if (app.rootId !== undefined) button.dataset.rootMenuId = String(app.rootId);
  if (app.parentPath) button.dataset.menuPath = app.parentPath;
  if (app.isMenuAction) button.dataset.menuAction = "true";
  button.title = app.name;
  button.setAttribute("aria-label", app.name);

  const name = document.createElement("strong");
  name.className = "o_caption o_app_name w-100 text-center text-truncate mt-2";
  name.textContent = app.name;
  button.append(appIconElement(app), name);

  if (app.parentPath) {
    const path = document.createElement("span");
    path.className = "o_app_menu_path";
    path.textContent = app.parentPath;
    button.append(path);
  }
  if (onClick) button.addEventListener("click", (event) => {
    event?.preventDefault?.();
    onClick();
  });
  wrapper.append(button);
  return wrapper;
}

const APPS_CATALOG_SEARCH_TEXT = "apps applications modules install";

function launcherRootApps(apps: readonly HomeMenuApp[], catalogApp: HomeMenuApp | null): HomeMenuApp[] {
  const settings = apps.find((app) => app.key === "settings" || app.name.toLowerCase() === "settings");
  const appsEntry = catalogApp ?? apps.find(isAppsCatalogApp) ?? apps.find((app) => app.key === "apps" || app.name.toLowerCase() === "apps");
  if (settings && appsEntry) return [
    { ...appsEntry, key: "apps", initials: "A", iconToken: "apps", parentPath: undefined },
    { ...settings, iconToken: "settings", parentPath: undefined }
  ];
  return [...apps];
}

function shouldAppendAppsCatalog(query: string, visible: readonly HomeMenuApp[], catalogApp: HomeMenuApp | null): boolean {
  if (!catalogApp) return false;
  if (visible.some((app) => app.key === "apps" || isAppsCatalogApp(app))) return false;
  if (!query) return true;
  return APPS_CATALOG_SEARCH_TEXT.includes(query) || catalogApp.searchText.includes(query);
}

function appHref(app: HomeMenuApp): string {
  const actionID = app.menu.actionID || app.menu.actionId || app.menu.directActionID;
  const menuID = encodeURIComponent(String(app.id));
  if (actionID) return `#menu_id=${menuID}&action=${encodeURIComponent(String(actionID))}`;
  return `#menu_id=${menuID}`;
}

function appIconImage(app: HomeMenuApp): HTMLImageElement | null {
  const generatedCoreIcon = cleanRoomLauncherIconSource(app);
  if (generatedCoreIcon) {
    const image = document.createElement("img");
    image.className = "o_app_icon o_app_icon_core rounded-3";
    image.alt = "";
    image.src = generatedCoreIcon;
    image.dataset.iconKind = launcherIconKind(app);
    image.dataset.iconToken = app.iconToken;
    image.setAttribute("aria-hidden", "true");
    image.dataset.generatedIcon = "clean-room";
    return image;
  }
  const source = typeof app.menu.webIconData === "string" ? app.menu.webIconData.trim() : "";
  const iconSource = appIconSource(source, app.menu.webIconDataMimetype);
  if (!iconSource) return null;
  const image = document.createElement("img");
  image.className = "o_app_icon rounded-3";
  image.alt = "";
  image.src = iconSource;
  image.dataset.iconKind = launcherIconKind(app);
  image.setAttribute("aria-hidden", "true");
  if (typeof app.menu.webIconDataMimetype === "string" && app.menu.webIconDataMimetype.trim()) {
    image.dataset.mimetype = app.menu.webIconDataMimetype.trim();
  }
  return image;
}

function appIconElement(app: HomeMenuApp): HTMLElement {
  const image = appIconImage(app);
  if (image) return image;
  const icon = document.createElement("span");
  icon.className = "o_app_icon o_app_icon_fallback position-relative d-flex justify-content-center align-items-center p-2 rounded-3";
  icon.dataset.iconToken = app.iconToken;
  icon.dataset.iconKind = launcherIconKind(app);
  icon.setAttribute("aria-hidden", "true");
  const webIcon = appWebIcon(app);
  if (webIcon) {
    icon.className = "o_app_icon o_app_icon_with_glyph position-relative d-flex justify-content-center align-items-center p-2 rounded-3";
    icon.dataset.webIcon = webIcon.iconClass;
    icon.dataset.iconKind = launcherIconKind(app);
    icon.setAttribute("style", `background-color: ${webIcon.backgroundColor}; --app-icon-bg: ${webIcon.backgroundColor}; color: ${webIcon.color};`);
    const glyph = document.createElement("i");
    glyph.className = `${webIcon.iconClass} o_app_icon_glyph`;
    icon.append(glyph);
  }
  return icon;
}

function cleanRoomLauncherIconSource(app: HomeMenuApp): string {
  const kind = launcherIconKind(app);
  if (kind === "apps") {
    return svgDataUri(`
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 70 70">
        <rect width="70" height="70" rx="5" fill="#263445"/>
        <path d="M35 35V10a25 25 0 0 1 25 25Z" fill="#b9639d"/>
        <path d="M35 35H10a25 25 0 0 1 25-25Z" fill="#2fc6bd"/>
        <path d="M35 35v25a25 25 0 0 1-25-25Z" fill="#ef5350"/>
        <path d="M35 35h25a25 25 0 0 1-25 25Z" fill="#35a6d9"/>
        <rect x="32.5" y="10" width="5" height="50" rx="2.5" fill="#263445"/>
        <rect x="10" y="32.5" width="50" height="5" rx="2.5" fill="#263445"/>
      </svg>
    `);
  }
  if (kind === "settings") {
    return svgDataUri(`
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 70 70">
        <rect width="70" height="70" rx="5" fill="#263445"/>
        <path d="M35 12 56 24v22L35 58 14 46V24Z" fill="#ee7f3f"/>
        <path d="M35 12 56 24v22L35 58Z" fill="#f6bd4f"/>
        <path d="M14 24 35 36 56 24" fill="none" stroke="#ff9c49" stroke-width="5" stroke-linejoin="round"/>
        <circle cx="35" cy="35" r="8" fill="#263445"/>
      </svg>
    `);
  }
  if (kind === "approvals") {
    return svgDataUri(`
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 70 70">
        <rect width="70" height="70" rx="7" fill="#6e7da4"/>
        <circle cx="35" cy="35" r="23" fill="#6e7da4"/>
        <path d="M24 35h22" stroke="#ffffff" stroke-width="6" stroke-linecap="round"/>
        <path d="M35 24v22" stroke="#263445" stroke-width="6" stroke-linecap="round"/>
        <circle cx="35" cy="35" r="12" fill="none" stroke="#ffffff" stroke-width="5"/>
      </svg>
    `);
  }
  if (kind === "delegation") {
    return svgDataUri(`
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 70 70">
        <rect width="70" height="70" rx="7" fill="#017e84"/>
        <rect x="14" y="14" width="34" height="34" rx="10" fill="#19a0a0"/>
        <rect x="24" y="22" width="30" height="30" rx="9" fill="#dfeff0"/>
        <rect x="43" y="20" width="8" height="32" rx="4" fill="#8fc9c2"/>
      </svg>
    `);
  }
  return "";
}

function svgDataUri(svg: string): string {
  return `data:image/svg+xml,${encodeURIComponent(svg.replace(/\s+/g, " ").trim())}`;
}

interface ParsedWebIcon {
  iconClass: string;
  color: string;
  backgroundColor: string;
}

function launcherIconKind(app: HomeMenuApp): string {
  const key = app.key.split(":")[0] || appKey(app.name);
  if (key === "apps" || key === "settings" || key === "approvals" || key === "delegation") return key;
  return "generated";
}

function appWebIcon(app: HomeMenuApp): ParsedWebIcon | null {
  const key = app.key.split(":")[0];
  const name = app.name.toLowerCase();
  if (key === "settings" || key === "apps" || name === "settings" || name === "apps") return null;
  const source = typeof app.menu.webIcon === "string" ? app.menu.webIcon.trim() : "";
  const parts = source.split(",").map((part) => part.trim());
  if (parts.length < 3) return defaultAppWebIcon(app);
  const iconClass = appIconClass(parts[0]);
  const color = safeIconColor(parts[1]) || "#ffffff";
  const backgroundColor = safeIconColor(parts[2]) || defaultAppIconBackground(app);
  if (!iconClass) return defaultAppWebIcon(app);
  return { iconClass, color, backgroundColor };
}

function defaultAppWebIcon(app: HomeMenuApp): ParsedWebIcon | null {
  const key = app.key.split(":")[0];
  const name = app.name.toLowerCase();
  const iconClass =
    key === "settings" || name.includes("setting") ? "" :
    key === "apps" || name === "apps" ? "" :
    name.includes("technical") ? "fa fa-cog" :
    name.includes("approval") ? "fa fa-check-square-o" :
    name.includes("delegation") ? "fa fa-exchange" :
    "";
  if (!iconClass) return null;
  return { iconClass, color: "#ffffff", backgroundColor: defaultAppIconBackground(app) };
}

function appIconClass(value: string): string {
  const raw = value.trim();
  if (!/^fa(?:\s+fa-[a-z0-9_-]+|-[a-z0-9_-]+)(?:\s+fa-[a-z0-9_-]+)*$/i.test(raw)) return "";
  return raw.startsWith("fa ") ? raw : `fa ${raw}`;
}

function safeIconColor(value: string): string {
  const color = value.trim();
  return /^#[0-9a-f]{3}(?:[0-9a-f]{3})?$/i.test(color) ? color : "";
}

function defaultAppIconBackground(app: HomeMenuApp): string {
  switch (app.iconToken) {
    case "apps": return "#b05f4a";
    case "settings": return "#5f6f94";
    case "teal": return "#017e84";
    case "purple": return "#875a7b";
    case "blue": return "#5f6f94";
    case "terracotta": return "#b05f4a";
    case "green": return "#228b65";
    case "slate": return "#56616f";
    default: return "#875a7b";
  }
}

function appIconSource(source: string, mimetype: unknown): string {
  if (!source || source.endsWith("/default_icon_app.png")) return "";
  if (/^(data:|https?:\/\/|\/)/.test(source)) return source;
  const mediaType = typeof mimetype === "string" && mimetype.trim() ? mimetype.trim() : "image/png";
  if (source.length >= 32 && /^[A-Za-z0-9+/=]+$/.test(source)) {
    return `data:${mediaType};base64,${source}`;
  }
  return "";
}
