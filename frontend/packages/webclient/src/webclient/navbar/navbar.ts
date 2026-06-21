export interface NavbarApp {
  id: number | string;
  name: string;
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
  debug?: boolean;
  systray?: NavbarSystrayState;
  activeAppId?: number | string;
  onOpenApps?: () => void;
  onOpenApp?: (app: NavbarApp) => void;
  onToggleMobileMenu?: (expanded: boolean) => void;
  onSystrayAction?: (action: NavbarSystrayAction) => void;
}

export interface RenderedNavbar extends HTMLElement {
  setActiveApp: (appId?: number | string) => void;
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
  header.className = "o_main_navbar d-print-none";
  let setMobileMenuExpanded = (_expanded: boolean) => {};
  const appButtons = new Map<string, HTMLElement>();
  const dropdowns: HTMLElement[] = [];
  const dropdownButtons = new Map<HTMLElement, HTMLElement>();

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
    setActiveApp(undefined);
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
  nav.className = "o-nav o_navbar_sections";
  nav.setAttribute("aria-label", "Application");
  for (const app of options.apps ?? []) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = app.name;
    button.className = "o_nav_entry";
    button.dataset.menuId = String(app.id);
    button.title = app.name;
    appButtons.set(String(app.id), button);
    button.addEventListener("click", () => {
      setMobileMenuExpanded(false);
      setActiveApp(app.id);
      options.onOpenApp?.(app);
    });
    nav.append(button);
  }

  const systray = document.createElement("div");
  systray.className = "o-menu-systray o_menu_systray d-flex flex-shrink-0 ms-auto bg-inherit";
  systray.setAttribute("role", "menu");
  systray.setAttribute("aria-label", "Systray");
  if (options.debug) appendDropdown(systray, renderDebugItem(), renderSystrayMenu("debug", debugMenuItems(), options.onSystrayAction));
  for (const item of defaultSystrayItems(options.systray?.store)) appendDropdown(systray, renderSystrayItem(item), renderSystrayMenu(item.key, item.menuItems ?? [item.label], options.onSystrayAction));
  appendDropdown(systray, renderCompanySwitcher(options.companyName ?? "My Company"), renderSystrayMenu("company", companyMenuItems(options.systray, options.companyName ?? "My Company"), options.onSystrayAction));
  appendDropdown(systray, renderUserMenu(options.userName ?? "Administrator"), renderSystrayMenu("user", userMenuItems(), options.onSystrayAction));

  header.append(brand, mobileMenu, nav, systray);
  bindSystrayAutoClose(header, closeDropdowns);
  header.setActiveApp = setActiveApp;
  setActiveApp(options.activeAppId);
  return header;

  function setActiveApp(appId?: number | string): void {
    const activeKey = appId === undefined || appId === null ? "" : String(appId);
    if (activeKey) {
      header.dataset.activeMenuId = activeKey;
    } else {
      delete header.dataset.activeMenuId;
    }
    const activeName = activeAppName(options.apps ?? [], appId);
    title.textContent = activeName ?? "Odoo";
    launcher.className = activeKey ? "o_menu_toggle o-launcher-button border-0" : "o_menu_toggle o-launcher-button border-0 active";
    setPageCurrent(launcher, !activeKey);
    for (const [key, button] of appButtons) {
      const active = key === activeKey;
      button.className = active ? "o_nav_entry active" : "o_nav_entry";
      setPageCurrent(button, active);
    }
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
      setDropdownOpen(button, menu, open);
    });
    parent.append(button, menu);
  }

  function closeDropdowns(except?: HTMLElement): void {
    for (const menu of dropdowns) {
      if (menu === except) continue;
      setDropdownOpen(dropdownButtons.get(menu) ?? null, menu, false);
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
  button.className = "o-mobile-menu-toggle";
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
    button.setAttribute("role", "menuitem");
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
  const base = menu.dataset.systrayDropdown ? `dropdown-menu o-dropdown-menu ${systrayMenuClass(menu.dataset.systrayDropdown)}`.trim() : "dropdown-menu o-dropdown-menu";
  menu.className = open ? `${base} show` : base;
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

function renderDebugItem(): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_debug_manager dropdown-toggle";
  button.setAttribute("role", "menuitem");
  button.textContent = "Debug";
  return button;
}

function renderUserMenu(userName: string): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_user_menu o-user-menu-button dropdown-toggle";
  button.setAttribute("aria-label", "User menu");
  button.setAttribute("role", "menuitem");
  const label = document.createElement("span");
  label.textContent = userName;
  button.append(label);
  return button;
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

function companyMenuItems(systray: NavbarSystrayState | undefined, fallbackName: string): SystrayMenuEntry[] {
  const companies = systray?.companies ?? [];
  if (!companies.length) {
    return [
      { label: fallbackName, active: true, action: { type: "switch-company" } },
      { label: "Switch Company", action: { type: "switch-company" } }
    ];
  }
  const currentKey = systray?.currentCompanyId === undefined || systray?.currentCompanyId === null ? "" : String(systray.currentCompanyId);
  const items = companies.map((company): SystrayMenuEntry => ({
    label: company.name,
    active: company.current || (!!currentKey && String(company.id) === currentKey),
    action: { type: "switch-company", companyId: company.id }
  }));
  if (systray?.displaySwitchCompanyMenu !== false && companies.length > 1) {
    items.push({ label: "Switch Company", action: { type: "switch-company" } });
  }
  return items;
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
