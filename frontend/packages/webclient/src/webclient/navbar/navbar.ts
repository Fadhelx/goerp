export interface NavbarApp {
  id: number | string;
  name: string;
  children?: readonly NavbarApp[];
  action?: boolean;
}

export interface SystrayItem {
  key: string;
  label: string;
  count?: number;
  className: string;
  menuItems?: readonly SystrayMenuEntry[];
}

export interface SystrayMenuItem {
  label: string;
  count?: number;
  description?: string;
  active?: boolean;
  action?: NavbarSystrayAction;
}

export type SystrayMenuEntry = string | SystrayMenuItem;

export interface NavbarSystrayAction {
  type: string;
  [key: string]: unknown;
}

export interface NavbarSystrayCompany {
  id: number | string;
  name: string;
  current?: boolean;
  active?: boolean;
}

export interface NavbarSystrayState {
  store?: Record<string, unknown>;
  companies?: readonly NavbarSystrayCompany[];
  currentCompanyId?: number | string;
  displaySwitchCompanyMenu?: boolean;
}

export interface NavbarOptions {
  apps?: readonly NavbarApp[];
  userName?: string;
  companyName?: string;
  databaseName?: string;
  debug?: boolean;
  systray?: NavbarSystrayState;
  activeAppId?: number | string;
  onOpenApps?: () => void;
  onOpenApp?: (app: NavbarApp) => void;
  onToggleMobileMenu?: (expanded: boolean) => void;
  onSystrayAction?: (action: NavbarSystrayAction) => void;
}

export interface RenderedNavbar extends HTMLElement {
  setActiveApp: (appId?: number | string, brandName?: string) => void;
  setApps: (apps: readonly NavbarApp[], activeAppId?: number | string, brandName?: string) => void;
  setHomeMenuBackMode: (enabled: boolean) => void;
}

export function defaultSystrayItems(store?: Record<string, unknown>): SystrayItem[] {
  const inboxCount = mailboxCounter(recordValue(store?.inbox));
  const starredCount = mailboxCounter(recordValue(store?.starred));
  const activityCount = numberValue(store?.activityCounter);
  const activityGroups = activityMenuItems(arrayValue(store?.activityGroups));
  return [
    {
      key: "messages",
      label: "Messages",
      count: inboxCount,
      className: "o_mail_systray_item",
      menuItems: [
        { label: "Inbox", count: inboxCount, action: { type: "open-mailbox", mailbox: "inbox" } },
        { label: "Starred", count: starredCount, action: { type: "open-mailbox", mailbox: "starred" } },
        { label: "New Message", action: { type: "new-message" } }
      ]
    },
    {
      key: "activities",
      label: "Activities",
      count: activityCount,
      className: "o_activity_menu",
      menuItems: activityGroups.length ? activityGroups : [{ label: "No activities", count: 0, action: { type: "open-activities" } }]
    }
  ];
}

export function renderNavbar(options: NavbarOptions = {}): RenderedNavbar {
  const header = document.createElement("header") as RenderedNavbar;
  header.className = "o_navbar";
  const mainNavbar = document.createElement("nav");
  mainNavbar.className = "o_main_navbar d-print-none";
  let setMobileMenuExpanded = (_expanded: boolean) => {};
  const appButtons = new Map<string, HTMLElement>();
  const appButtonEntries = new Map<string, NavbarApp>();
  const dropdowns: HTMLElement[] = [];
  const dropdownButtons = new Map<HTMLElement, HTMLElement>();
  const submenuPairs: Array<{ toggle: HTMLElement; menu: HTMLElement; entry: NavbarApp }> = [];
  let currentApps = [...(options.apps ?? [])];
  let homeMenuBackMode = false;

  const brand = document.createElement("div");
  brand.className = "o_navbar_apps_menu o-brand";
  const launcher = document.createElement("button");
  launcher.type = "button";
  launcher.className = "o_menu_toggle o-launcher-button border-0";
  launcher.dataset.view = "apps";
  launcher.setAttribute("aria-label", "Apps");
  launcher.setAttribute("accesskey", "h");
  launcher.append(renderLauncherIcon());
  launcher.addEventListener("click", () => {
    setMobileMenuExpanded(false);
    options.onOpenApps?.();
  });
  const title = document.createElement("h1");
  title.className = "o_menu_brand";
  title.textContent = activeAppName(options.apps ?? [], options.activeAppId) ?? "Odoo";
  brand.append(launcher, title);

  const mobileMenu = renderMobileMenuToggle((expanded) => {
    options.onToggleMobileMenu?.(expanded);
  });
  setMobileMenuExpanded = (expanded: boolean) => {
    mobileMenu.setAttribute("aria-expanded", expanded ? "true" : "false");
    options.onToggleMobileMenu?.(expanded);
  };

  const nav = document.createElement("nav");
  nav.className = "o-nav o_navbar_sections o_menu_sections";
  nav.setAttribute("aria-label", "Application");
  renderNavEntries(currentApps);

  function renderNavEntries(apps: readonly NavbarApp[]): void {
    nav.replaceChildren?.();
    appButtons.clear();
    appButtonEntries.clear();
    submenuPairs.splice(0);
    for (const app of apps) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = app.name;
      button.className = app.children?.length ? "o_nav_entry o_nav_dropdown_toggle dropdown-toggle" : "o_nav_entry";
      button.dataset.menuId = String(app.id);
      button.title = app.name;
      appButtons.set(String(app.id), button);
      appButtonEntries.set(String(app.id), app);
      if (app.children?.length) {
        appendDropdown(nav, button, renderNavbarDropdownMenu(app));
      } else {
        button.addEventListener("click", () => {
          setMobileMenuExpanded(false);
          setActiveApp(app.id);
          options.onOpenApp?.(app);
        });
        nav.append(button);
      }
    }
  }

  const systray = document.createElement("div");
  systray.className = "o-menu-systray o_menu_systray d-flex flex-shrink-0 ms-auto bg-inherit";
  systray.setAttribute("role", "menu");
  systray.setAttribute("aria-label", "Systray");
  if (options.debug) {
    appendDropdown(systray, renderDebugItem(), renderSystrayMenu("debug", debugMenuItems(), options.onSystrayAction));
    appendDropdown(systray, renderDebugToolsItem(), renderSystrayMenu("debug-tools", debugToolsMenuItems(), options.onSystrayAction));
  }
  for (const item of defaultSystrayItems(options.systray?.store)) appendDropdown(systray, renderSystrayItem(item), renderSystrayMenu(item.key, item.menuItems ?? [item.label], options.onSystrayAction));
  appendDropdown(systray, renderCompanySwitcher(options.companyName ?? "My Company"), renderCompanySwitcherMenu(options.systray, options.companyName ?? "My Company", options.onSystrayAction));
  appendDropdown(systray, renderUserMenu(options.userName ?? "Administrator", options.databaseName), renderSystrayMenu("user", userMenuItems(), options.onSystrayAction));

  mainNavbar.append(brand, mobileMenu, nav, systray);
  header.append(mainNavbar);
  bindSystrayAutoClose(header, closeDropdowns);
  header.setActiveApp = setActiveApp;
  header.setApps = setApps;
  header.setHomeMenuBackMode = setHomeMenuBackMode;
  setActiveApp(options.activeAppId);
  return header;

  function setApps(apps: readonly NavbarApp[], activeAppId?: number | string, brandName?: string): void {
    currentApps = [...apps];
    renderNavEntries(currentApps);
    setActiveApp(activeAppId, brandName);
  }

  function setActiveApp(appId?: number | string, brandName?: string): void {
    const activeKey = appId === undefined || appId === null ? "" : String(appId);
    if (activeKey) {
      header.dataset.activeMenuId = activeKey;
      mainNavbar.dataset.activeMenuId = activeKey;
    } else {
      delete header.dataset.activeMenuId;
      delete mainNavbar.dataset.activeMenuId;
    }
    const activeName = activeAppName(currentApps, appId);
    title.textContent = brandName || activeName || "Odoo";
    updateLauncherClass(Boolean(activeKey));
    setPageCurrent(launcher, !activeKey);
    for (const [key, button] of appButtons) {
      const entry = appButtonEntries.get(key);
      const dropdown = Boolean(entry?.children?.length);
      const active = key === activeKey || Boolean(entry && activeKey && menuContains(entry, activeKey));
      if (String(button.className ?? "").split(/\s+/).includes("o_navbar_dropdown_item")) {
        const classes = ["dropdown-item", "o_navbar_dropdown_item"];
        if (String(button.className ?? "").split(/\s+/).includes("o_navbar_submenu_toggle")) {
          classes.push("o_navbar_submenu_toggle", "dropdown-toggle");
        }
        if (active) classes.push("active");
        button.className = classes.join(" ");
        setPageCurrent(button, active);
        continue;
      }
      const classes = dropdown ? ["o_nav_entry", "o_nav_dropdown_toggle", "dropdown-toggle"] : ["o_nav_entry"];
      if (active) classes.push("active");
      button.className = classes.join(" ");
      setPageCurrent(button, active);
    }
  }

  function renderNavbarDropdownMenu(rootApp: NavbarApp): HTMLElement {
    const menu = document.createElement("div");
    menu.className = "dropdown-menu o-dropdown-menu o_navbar_dropdown_menu";
    menu.dataset.navbarDropdown = String(rootApp.id);
    menu.hidden = true;
    menu.setAttribute("role", "menu");
    appendNavbarMenuEntries(menu, rootApp.children ?? [], 0);
    return menu;
  }

  function appendNavbarMenuEntries(menu: HTMLElement, entries: readonly NavbarApp[], level: number): void {
    for (const entry of entries) {
      if (entry.children?.length && entry.action === false) {
        appendNavbarMenuHeader(menu, entry, level);
        appendNavbarMenuEntries(menu, entry.children, level + 1);
      } else if (entry.children?.length) {
        const group = document.createElement("div");
        group.className = "o_navbar_dropdown_group";
        group.dataset.menuId = String(entry.id);
        group.dataset.menuLevel = String(level);
        group.setAttribute("role", "none");

        const toggle = document.createElement("button");
        toggle.type = "button";
        toggle.className = "dropdown-item o_navbar_dropdown_item o_navbar_submenu_toggle dropdown-toggle";
        toggle.dataset.menuId = String(entry.id);
        toggle.dataset.menuLevel = String(level);
        toggle.setAttribute("role", "menuitem");
        toggle.setAttribute("aria-haspopup", "menu");
        toggle.setAttribute("aria-expanded", "false");
        toggle.textContent = entry.name;
        appButtons.set(String(entry.id), toggle);
        appButtonEntries.set(String(entry.id), entry);

        const submenu = document.createElement("div");
        submenu.className = "dropdown-menu o-dropdown-menu o_navbar_submenu_menu";
        submenu.dataset.navbarSubmenu = String(entry.id);
        submenu.hidden = true;
        submenu.setAttribute("role", "menu");
        appendNavbarMenuEntries(submenu, entry.children, level + 1);

        toggle.addEventListener("click", (event) => {
          event.stopPropagation?.();
          event.preventDefault?.();
          const open = toggle.getAttribute("aria-expanded") !== "true";
          closeSiblingSubmenus(group);
          setSubmenuOpen(toggle, submenu, open);
        });
        toggle.addEventListener("keydown", (event) => {
          if (event.key === "ArrowRight") {
            event.preventDefault();
            setSubmenuOpen(toggle, submenu, true);
          } else if (event.key === "Escape") {
            event.preventDefault();
            setSubmenuOpen(toggle, submenu, false);
          }
        });

        submenuPairs.push({ toggle, menu: submenu, entry });
        group.append(toggle, submenu);
        menu.append(group);
      } else if (entry.action !== false) {
        const item = document.createElement("button");
        item.type = "button";
        item.className = "dropdown-item o_navbar_dropdown_item";
        item.dataset.menuId = String(entry.id);
        item.dataset.menuLevel = String(level);
        item.setAttribute("role", "menuitem");
        item.textContent = entry.name;
        appButtons.set(String(entry.id), item);
        appButtonEntries.set(String(entry.id), entry);
        item.addEventListener("click", () => {
          setMobileMenuExpanded(false);
          closeDropdowns();
          setActiveApp(entry.id);
          options.onOpenApp?.(entry);
        });
        menu.append(item);
      } else {
        appendNavbarMenuHeader(menu, entry, level);
      }
    }
  }

  function appendNavbarMenuHeader(menu: HTMLElement, entry: NavbarApp, level: number): void {
    const header = document.createElement("span");
    header.className = "dropdown-header o_navbar_dropdown_header";
    header.dataset.menuId = String(entry.id);
    header.dataset.menuLevel = String(level);
    header.textContent = entry.name;
    menu.append(header);
  }

  function setHomeMenuBackMode(enabled: boolean): void {
    homeMenuBackMode = enabled;
    updateLauncherClass(Boolean(mainNavbar.dataset.activeMenuId));
  }

  function updateLauncherClass(hasActiveApp: boolean): void {
    const classes = ["o_menu_toggle", "o-launcher-button", "border-0"];
    if (!hasActiveApp) classes.push("active");
    if (homeMenuBackMode) classes.push("o_menu_toggle_back");
    launcher.className = classes.join(" ");
  }

  function appendDropdown(parent: HTMLElement, button: HTMLElement, menu: HTMLElement): void {
    dropdowns.push(menu);
    dropdownButtons.set(menu, button);
    button.setAttribute("aria-haspopup", "menu");
    button.setAttribute("aria-expanded", "false");
    button.addEventListener("click", (event) => {
      event.stopPropagation?.();
      const open = button.getAttribute("aria-expanded") !== "true";
      closeDropdowns(menu);
      if (open) mountDropdownMenu(parent, button, menu);
      setDropdownOpen(button, menu, open);
      if (open) openActiveSubmenus(mainNavbar.dataset.activeMenuId ?? "");
      if (!open) unmountDropdownMenu(menu);
    });
    parent.append(button);
  }

  function closeDropdowns(except?: HTMLElement): void {
    closeAllSubmenus();
    for (const menu of dropdowns) {
      if (menu === except) continue;
      setDropdownOpen(dropdownButtons.get(menu) ?? null, menu, false);
      unmountDropdownMenu(menu);
    }
  }

  function closeAllSubmenus(): void {
    for (const pair of submenuPairs) setSubmenuOpen(pair.toggle, pair.menu, false);
  }

  function closeSiblingSubmenus(group: HTMLElement): void {
    const parent = group.parentNode as HTMLElement | null;
    if (!parent) return;
    for (const sibling of Array.from(parent.children ?? [])) {
      if (sibling === group) continue;
      const element = sibling as HTMLElement;
      const toggle = firstChildWithClass(element, "o_navbar_submenu_toggle");
      const menu = firstChildWithClass(element, "o_navbar_submenu_menu");
      if (toggle && menu) setSubmenuOpen(toggle, menu, false);
    }
  }

  function openActiveSubmenus(activeKey: string): void {
    if (!activeKey) return;
    for (const pair of submenuPairs) {
      if (menuContains(pair.entry, activeKey)) setSubmenuOpen(pair.toggle, pair.menu, true);
    }
  }
}

function renderLauncherIcon(): HTMLElement {
  const icon = document.createElement("span");
  icon.className = "o_menu_toggle_icon o-launcher";
  icon.setAttribute("aria-hidden", "true");
  for (let index = 0; index < 9; index += 1) icon.append(document.createElement("span"));
  return icon;
}

function renderMobileMenuToggle(onToggle: (expanded: boolean) => void): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-mobile-menu-toggle o_mobile_menu_toggle";
  button.setAttribute("aria-label", "Menu");
  button.setAttribute("aria-expanded", "false");
  const line = document.createElement("span");
  line.setAttribute("aria-hidden", "true");
  button.append(line);
  button.addEventListener("click", () => {
    const expanded = button.getAttribute("aria-expanded") !== "true";
    button.setAttribute("aria-expanded", expanded ? "true" : "false");
    onToggle(expanded);
  });
  return button;
}

function renderSystrayItem(item: SystrayItem): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `o-systray-item ${item.className} dropdown-toggle`;
  button.setAttribute("aria-label", item.label);
  button.setAttribute("role", "menuitem");
  const icon = document.createElement("i");
  icon.className = item.key === "activities" ? "o-systray-icon oi oi-clock" : "o-systray-icon oi oi-discuss";
  icon.setAttribute("aria-label", item.label);
  icon.setAttribute("title", item.label);
  const counter = document.createElement("span");
  counter.className = "o-systray-counter";
  counter.textContent = String(item.count ?? 0);
  counter.hidden = (item.count ?? 0) <= 0;
  button.append(icon, counter);
  return button;
}

function renderSystrayMenu(key: string, items: readonly SystrayMenuEntry[], onAction?: (action: NavbarSystrayAction) => void): HTMLElement {
  const menu = document.createElement("div");
  menu.className = `dropdown-menu o-dropdown-menu ${systrayMenuClass(key)}`.trim();
  menu.dataset.systrayDropdown = key;
  menu.hidden = true;
  menu.setAttribute("role", "menu");
  for (const item of items) {
    const entry = normalizeMenuEntry(item);
    const button = document.createElement("button");
    button.type = "button";
    button.className = entry.active ? "dropdown-item active" : "dropdown-item";
    button.setAttribute("role", "none");
    button.dataset.systrayItem = entry.label;
    if (entry.action) {
      button.dataset.systrayAction = entry.action.type;
      button.addEventListener("click", () => onAction?.(entry.action as NavbarSystrayAction));
    }
    const label = document.createElement("span");
    label.className = "o_systray_menu_label";
    label.textContent = entry.label;
    button.append(label);
    if (typeof entry.count === "number") {
      const badge = document.createElement("span");
      badge.className = "o_systray_menu_badge";
      badge.textContent = String(entry.count);
      button.append(badge);
    }
    if (entry.description) {
      const description = document.createElement("span");
      description.className = "o_systray_menu_description";
      description.textContent = entry.description;
      button.append(description);
    }
    menu.append(button);
  }
  return menu;
}

function systrayMenuClass(key: string): string {
  switch (key) {
    case "messages":
      return "o-mail-MessagingMenu";
    case "activities":
      return "o-mail-ActivityMenu";
    case "company":
      return "o_switch_company_menu_dropdown";
    default:
      return "";
  }
}

function normalizeMenuEntry(entry: SystrayMenuEntry): SystrayMenuItem {
  if (typeof entry === "string") return { label: entry };
  return entry;
}

function setDropdownOpen(button: HTMLElement | null, menu: HTMLElement, open: boolean): void {
  if (button) button.setAttribute("aria-expanded", open ? "true" : "false");
  menu.hidden = !open;
  const base = menu.dataset.systrayDropdown
    ? `dropdown-menu o-dropdown-menu ${systrayMenuClass(menu.dataset.systrayDropdown)}`.trim()
    : menu.dataset.navbarDropdown
      ? "dropdown-menu o-dropdown-menu o_navbar_dropdown_menu"
      : "dropdown-menu o-dropdown-menu";
  menu.className = open ? `${base} show` : base;
  positionNavbarDropdown(button, menu, open);
}

function mountDropdownMenu(parent: HTMLElement, button: HTMLElement, menu: HTMLElement): void {
  if (menu.parentNode) return;
  const buttonParent = button.parentNode;
  if (buttonParent && typeof buttonParent.insertBefore === "function") {
    buttonParent.insertBefore(menu, button.nextSibling);
    return;
  }
  parent.append(menu);
}

function unmountDropdownMenu(menu: HTMLElement): void {
  if (!menu.parentNode) return;
  if (typeof menu.remove === "function") {
    menu.remove();
    return;
  }
  const parent = menu.parentNode;
  if (typeof parent.removeChild === "function") parent.removeChild(menu);
}

function positionNavbarDropdown(button: HTMLElement | null, menu: HTMLElement, open: boolean): void {
  if (!menu.dataset.navbarDropdown) return;
  if (!menu.style) return;
  if (!open || !button || typeof button.getBoundingClientRect !== "function") {
    menu.style.removeProperty("position");
    menu.style.removeProperty("top");
    menu.style.removeProperty("left");
    menu.style.removeProperty("right");
    menu.style.removeProperty("z-index");
    return;
  }
  const rect = button.getBoundingClientRect();
  const width = Math.max(menu.offsetWidth || 230, 230);
  const viewportWidth = globalThis.innerWidth || document.documentElement.clientWidth || width;
  const left = Math.min(Math.max(rect.left, 8), Math.max(8, viewportWidth - width - 8));
  menu.style.position = "fixed";
  menu.style.top = `${Math.round(rect.bottom)}px`;
  menu.style.left = `${Math.round(left)}px`;
  menu.style.right = "auto";
  menu.style.zIndex = "2055";
}

function setSubmenuOpen(button: HTMLElement, menu: HTMLElement, open: boolean): void {
  button.setAttribute("aria-expanded", open ? "true" : "false");
  menu.hidden = !open;
  const base = "dropdown-menu o-dropdown-menu o_navbar_submenu_menu";
  menu.className = open ? `${base} show` : base;
}

function firstChildWithClass(root: HTMLElement, className: string): HTMLElement | null {
  for (const child of Array.from(root.children ?? [])) {
    const element = child as HTMLElement;
    if (String(element.className ?? "").split(/\s+/).includes(className)) return element;
  }
  return null;
}

function menuContains(entry: NavbarApp, menuId: string): boolean {
  if (String(entry.id) === menuId) return true;
  return Boolean(entry.children?.some((child) => menuContains(child, menuId)));
}

function bindSystrayAutoClose(root: HTMLElement, closeDropdowns: () => void): void {
  const doc = globalThis.document as Document & { addEventListener?: Document["addEventListener"] };
  if (typeof doc.addEventListener !== "function") return;
  doc.addEventListener("click", (event) => {
    if (typeof root.contains === "function" && !root.contains(event.target as Node)) closeDropdowns();
  });
  doc.addEventListener("keydown", (event) => {
    if (event.key === "Escape") closeDropdowns();
  });
}

function renderCompanySwitcher(companyName: string): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_switch_company_menu o-company-switcher dropdown-toggle";
  button.setAttribute("aria-label", "Company");
  button.setAttribute("role", "menuitem");
  const label = document.createElement("span");
  label.className = "oe_topbar_name";
  label.textContent = companyName;
  button.append(label);
  return button;
}

function renderCompanySwitcherMenu(systray: NavbarSystrayState | undefined, fallbackName: string, onAction?: (action: NavbarSystrayAction) => void): HTMLElement {
  const companies = systray?.companies ?? [];
  if (!companies.length) return renderFallbackCompanyMenu(fallbackName, onAction);

  const menu = document.createElement("div");
  menu.className = "dropdown-menu o-dropdown-menu o_switch_company_menu_dropdown";
  menu.dataset.systrayDropdown = "company";
  menu.hidden = true;
  menu.setAttribute("role", "menu");

  const initialPrimaryKey = currentCompanyKey(systray, companies);
  const initialSelectedKeys = selectedCompanyKeys(companies, initialPrimaryKey);
  let primaryKey = initialPrimaryKey;
  let selectedKeys = new Set(initialSelectedKeys);
  const rows: CompanySwitcherRow[] = [];
  const confirm = document.createElement("button") as HTMLButtonElement;
  const reset = document.createElement("button") as HTMLButtonElement;

  const updateSelection = () => {
    for (const row of rows) {
      const key = String(row.company.id);
      const selected = selectedKeys.has(key);
      const primary = key === primaryKey;
      row.item.className = selected ? "dropdown-item o_switch_company_item active" : "dropdown-item o_switch_company_item";
      row.item.setAttribute("aria-checked", selected ? "true" : "false");
      row.item.setAttribute("aria-pressed", primary ? "true" : "false");
      row.logInto.setAttribute("aria-pressed", primary ? "true" : "false");
    }
    confirm.disabled = selectedKeys.size === 0;
    reset.disabled = primaryKey === initialPrimaryKey && sameKeySet(selectedKeys, initialSelectedKeys);
  };
  const toggleCompany = (companyId: number | string) => {
    const key = String(companyId);
    if (selectedKeys.has(key)) {
      selectedKeys.delete(key);
    } else {
      selectedKeys.add(key);
    }
    if (!selectedKeys.has(primaryKey)) {
      primaryKey = selectedKeys.values().next().value ?? initialPrimaryKey;
    }
    updateSelection();
  };
  const actionForCompany = (company: NavbarSystrayCompany): NavbarSystrayAction => ({
    type: "switch-company",
    companyId: company.id,
    companyIds: orderedCompanyIDs(company.id, selectedCompanyIDs(companies, selectedKeys))
  });
  const confirmAction = (): NavbarSystrayAction | undefined => {
    const primary = selectedCompany(companies, primaryKey) ?? selectedCompany(companies, selectedKeys.values().next().value ?? "");
    if (!primary) return undefined;
    return {
      type: "switch-company",
      companyId: primary.id,
      companyIds: orderedCompanyIDs(primary.id, selectedCompanyIDs(companies, selectedKeys))
    };
  };

  if (companies.length > 9) {
    const search = document.createElement("input") as HTMLInputElement;
    search.type = "search";
    search.className = "o_switch_company_search";
    search.setAttribute("role", "searchbox");
    search.setAttribute("aria-label", "Search companies");
    search.addEventListener("input", () => {
      const query = normalizeCompanySearch(search.value);
      for (const row of rows) {
        row.item.hidden = query.length > 0 && !normalizeCompanySearch(row.company.name).includes(query);
      }
    });
    menu.append(search);
  }

  const list = document.createElement("div");
  list.className = "o_switch_company_menu_companies";
  for (const company of companies) {
    const item = document.createElement("div");
    item.className = "dropdown-item o_switch_company_item";
    item.dataset.companyId = String(company.id);
    item.dataset.systrayItem = company.name;
    item.setAttribute("role", "menuitemcheckbox");
    item.setAttribute("tabindex", "0");
    item.addEventListener("click", () => toggleCompany(company.id));
    item.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      toggleCompany(company.id);
    });

    const name = document.createElement("span");
    name.className = "o_switch_company_item_name";
    name.textContent = company.name;

    const logInto = document.createElement("button");
    logInto.type = "button";
    logInto.className = "log_into";
    logInto.dataset.companyId = String(company.id);
    logInto.dataset.systrayAction = "switch-company";
    logInto.setAttribute("role", "none");
    logInto.textContent = "Log into";
    logInto.addEventListener("click", (event) => {
      event.stopPropagation();
      if (!selectedKeys.has(String(company.id))) selectedKeys.add(String(company.id));
      onAction?.(actionForCompany(company));
    });

    item.append(name, logInto);
    rows.push({ company, item, logInto });
    list.append(item);
  }
  menu.append(list);

  const buttons = document.createElement("div");
  buttons.className = "o_switch_company_menu_buttons";
  confirm.type = "button";
  confirm.className = "btn btn-primary o_switch_company_confirm";
  confirm.dataset.systrayAction = "switch-company";
  confirm.setAttribute("role", "none");
  confirm.textContent = "Confirm";
  confirm.addEventListener("click", () => {
    const action = confirmAction();
    if (action) onAction?.(action);
  });
  reset.type = "button";
  reset.className = "btn btn-secondary o_switch_company_reset";
  reset.setAttribute("role", "none");
  reset.textContent = "Reset";
  reset.addEventListener("click", () => {
    primaryKey = initialPrimaryKey;
    selectedKeys = new Set(initialSelectedKeys);
    updateSelection();
  });
  buttons.append(confirm, reset);
  menu.append(buttons);

  updateSelection();
  return menu;
}

interface CompanySwitcherRow {
  company: NavbarSystrayCompany;
  item: HTMLElement;
  logInto: HTMLButtonElement;
}

function renderFallbackCompanyMenu(fallbackName: string, onAction?: (action: NavbarSystrayAction) => void): HTMLElement {
  const menu = document.createElement("div");
  menu.className = "dropdown-menu o-dropdown-menu o_switch_company_menu_dropdown";
  menu.dataset.systrayDropdown = "company";
  menu.hidden = true;
  menu.setAttribute("role", "menu");

  const item = document.createElement("button");
  item.type = "button";
  item.className = "dropdown-item o_switch_company_item active";
  item.dataset.systrayItem = fallbackName;
  item.setAttribute("role", "menuitemcheckbox");
  item.setAttribute("aria-checked", "true");
  item.setAttribute("aria-pressed", "true");
  item.addEventListener("click", () => onAction?.({ type: "switch-company" }));
  const label = document.createElement("span");
  label.className = "o_switch_company_item_name";
  label.textContent = fallbackName;
  item.append(label);

  const buttons = document.createElement("div");
  buttons.className = "o_switch_company_menu_buttons";
  const confirm = document.createElement("button");
  confirm.type = "button";
  confirm.className = "btn btn-primary o_switch_company_confirm";
  confirm.dataset.systrayAction = "switch-company";
  confirm.setAttribute("role", "none");
  confirm.textContent = "Confirm";
  confirm.addEventListener("click", () => onAction?.({ type: "switch-company" }));
  buttons.append(confirm);

  menu.append(item, buttons);
  return menu;
}

function currentCompanyKey(systray: NavbarSystrayState | undefined, companies: readonly NavbarSystrayCompany[]): string {
  const explicit = systray?.currentCompanyId;
  if (explicit !== undefined && explicit !== null) return String(explicit);
  const current = companies.find((company) => company.current);
  return current ? String(current.id) : "";
}

function selectedCompany(companies: readonly NavbarSystrayCompany[], selectedKey: string): NavbarSystrayCompany | undefined {
  return companies.find((company) => String(company.id) === selectedKey);
}

function selectedCompanyKeys(companies: readonly NavbarSystrayCompany[], primaryKey: string): Set<string> {
  const keys = new Set(companies.filter((company) => company.active || company.current).map((company) => String(company.id)));
  if (!keys.size && primaryKey) keys.add(primaryKey);
  return keys;
}

function selectedCompanyIDs(companies: readonly NavbarSystrayCompany[], keys: Set<string>): Array<number | string> {
  return companies.filter((company) => keys.has(String(company.id))).map((company) => company.id);
}

function orderedCompanyIDs(primary: number | string, ids: Array<number | string>): Array<number | string> {
  const primaryKey = String(primary);
  const out: Array<number | string> = [];
  const seen = new Set<string>();
  const push = (id: number | string) => {
    const key = String(id);
    if (seen.has(key)) return;
    seen.add(key);
    out.push(id);
  };
  push(primary);
  for (const id of ids) {
    if (String(id) !== primaryKey) push(id);
  }
  return out;
}

function sameKeySet(left: Set<string>, right: Set<string>): boolean {
  if (left.size !== right.size) return false;
  for (const key of left) {
    if (!right.has(key)) return false;
  }
  return true;
}

function normalizeCompanySearch(value: string): string {
  return value.toLowerCase().replace(/\s+/g, "");
}

function renderDebugItem(): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_debug_manager dropdown-toggle";
  button.setAttribute("aria-label", "Debug");
  button.setAttribute("role", "none");
  const icon = document.createElement("i");
  icon.className = "o-systray-icon o_debug_icon";
  icon.setAttribute("aria-hidden", "true");
  button.append(icon);
  return button;
}

function renderDebugToolsItem(): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_debug_tools dropdown-toggle";
  button.setAttribute("aria-label", "Developer tools");
  button.setAttribute("role", "menuitem");
  const icon = document.createElement("i");
  icon.className = "o-systray-icon o_debug_tools_icon";
  icon.setAttribute("aria-hidden", "true");
  button.append(icon);
  return button;
}

function renderUserMenu(userName: string, databaseName?: string): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_user_menu o-user-menu-button dropdown-toggle";
  button.setAttribute("aria-label", "User menu");
  button.setAttribute("role", "menuitem");
  const avatar = document.createElement("span");
  avatar.className = "o_user_avatar";
  avatar.setAttribute("aria-hidden", "true");
  avatar.textContent = userInitial(userName);
  const label = document.createElement("span");
  label.className = "o_user_menu_name";
  label.textContent = userName;
  button.append(avatar, label);
  if (databaseName?.trim()) {
    const database = document.createElement("span");
    database.className = "o_database_name";
    database.setAttribute("role", "menuitem");
    const databaseIcon = document.createElement("i");
    databaseIcon.className = "o_database_icon";
    databaseIcon.setAttribute("aria-hidden", "true");
    const databaseLabel = document.createElement("span");
    databaseLabel.className = "o_database_label";
    databaseLabel.textContent = databaseName.trim();
    database.append(databaseIcon, databaseLabel);
    button.append(database);
  }
  return button;
}

function userInitial(userName: string): string {
  const trimmed = userName.trim();
  return (trimmed ? trimmed[0] : "A").toUpperCase();
}

function userMenuItems(): SystrayMenuEntry[] {
  return [
    { label: "Help", action: { type: "open-help" } },
    { label: "Shortcuts", action: { type: "open-shortcuts" } },
    { label: "My Preferences", action: { type: "open-preferences" } },
    { label: "My Profile", action: { type: "open-profile" } },
    { label: "My Odoo.com Account", action: { type: "open-odoo-account" } },
    { label: "Log out", action: { type: "logout" } }
  ];
}

function debugMenuItems(): SystrayMenuEntry[] {
  return [
    { label: "Open Developer Tools", action: { type: "open-debug-tools" } },
    { label: "Metadata", action: { type: "view-metadata" } },
    { label: "Access Rights", action: { type: "view-access-rights" } },
    { label: "Record Rules", action: { type: "view-record-rules" } },
    { label: "Become Superuser", action: { type: "become-superuser" } },
    { label: "Leave Debug Mode", action: { type: "leave-debug-mode" } }
  ];
}

function debugToolsMenuItems(): SystrayMenuEntry[] {
  return [
    { label: "Open Developer Tools", action: { type: "open-debug-tools" } },
    { label: "View Metadata", action: { type: "view-metadata" } }
  ];
}

function activityMenuItems(groups: unknown[]): SystrayMenuEntry[] {
  return groups
    .map((raw): SystrayMenuEntry | undefined => {
      const group = recordValue(raw);
      if (!group) return undefined;
      const total = numberValue(group.total_count);
      const overdue = numberValue(group.overdue_count);
      const today = numberValue(group.today_count);
      const planned = numberValue(group.planned_count);
      const name = firstText(group.name, group.model, "Activities");
      return {
        label: name,
        count: total,
        description: `Late ${overdue} Today ${today} Future ${planned}`,
        action: {
          type: "open-activities",
          model: firstText(group.model),
          domain: group.domain,
          viewType: firstText(group.view_type, "list")
        }
      };
    })
    .filter((item): item is SystrayMenuEntry => item !== undefined);
}

function mailboxCounter(mailbox: Record<string, unknown> | undefined): number {
  return numberValue(mailbox?.counter);
}

function numberValue(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return Math.max(0, Math.trunc(value));
  if (typeof value === "string" && value.trim()) {
    const parsed = Number.parseInt(value, 10);
    if (Number.isFinite(parsed)) return Math.max(0, parsed);
  }
  return 0;
}

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : undefined;
}

function arrayValue(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function firstText(...values: unknown[]): string {
  for (const value of values) {
    const text = typeof value === "string" ? value.trim() : "";
    if (text) return text;
  }
  return "";
}

function activeAppName(apps: readonly NavbarApp[], appId: number | string | undefined): string | undefined {
  if (appId === undefined || appId === null) return undefined;
  return apps.find((app) => String(app.id) === String(appId))?.name;
}

function setPageCurrent(node: HTMLElement, current: boolean): void {
  if (current) {
    node.setAttribute("aria-current", "page");
  } else if (typeof node.removeAttribute === "function") {
    node.removeAttribute("aria-current");
  } else {
    node.setAttribute("aria-current", "false");
  }
}
