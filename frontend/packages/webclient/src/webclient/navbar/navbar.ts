export interface NavbarApp {
  id: number | string;
  name: string;
}

export interface SystrayItem {
  key: string;
  label: string;
  count?: number;
  className: string;
}

export interface NavbarOptions {
  apps?: readonly NavbarApp[];
  userName?: string;
  companyName?: string;
  debug?: boolean;
  activeAppId?: number | string;
  onOpenApps?: () => void;
  onOpenApp?: (app: NavbarApp) => void;
}

export function defaultSystrayItems(): SystrayItem[] {
  return [
    { key: "messages", label: "Messages", count: 0, className: "o_mail_systray_item" },
    { key: "activities", label: "Activities", count: 0, className: "o_activity_menu" }
  ];
}

export function renderNavbar(options: NavbarOptions = {}): HTMLElement {
  const header = document.createElement("header");
  header.className = "o_main_navbar d-print-none";

  const brand = document.createElement("div");
  brand.className = "o_navbar_apps_menu o-brand";
  const launcher = document.createElement("button");
  launcher.type = "button";
  launcher.className = "o_menu_toggle o-launcher-button border-0";
  launcher.dataset.view = "apps";
  launcher.setAttribute("aria-label", "Apps");
  launcher.setAttribute("accesskey", "h");
  launcher.append(renderLauncherIcon());
  launcher.addEventListener("click", () => options.onOpenApps?.());
  const title = document.createElement("h1");
  title.className = "o_menu_brand";
  title.textContent = "Odoo";
  brand.append(launcher, title);

  const nav = document.createElement("nav");
  nav.className = "o-nav o_navbar_sections";
  nav.setAttribute("aria-label", "Application");
  for (const app of options.apps ?? []) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = app.name;
    button.className = "o_nav_entry";
    if (String(app.id) === String(options.activeAppId ?? "")) button.className = "o_nav_entry active";
    button.addEventListener("click", () => options.onOpenApp?.(app));
    nav.append(button);
  }

  const systray = document.createElement("div");
  systray.className = "o-menu-systray o_menu_systray d-flex flex-shrink-0 ms-auto bg-inherit";
  systray.setAttribute("role", "menu");
  systray.setAttribute("aria-label", "Systray");
  for (const item of defaultSystrayItems()) systray.append(renderSystrayItem(item));
  systray.append(renderCompanySwitcher(options.companyName ?? "My Company"));
  if (options.debug) systray.append(renderDebugItem());
  systray.append(renderUserMenu(options.userName ?? "Administrator"));

  header.append(brand, nav, systray);
  return header;
}

function renderLauncherIcon(): HTMLElement {
  const icon = document.createElement("span");
  icon.className = "o_menu_toggle_icon o-launcher";
  icon.setAttribute("aria-hidden", "true");
  for (let index = 0; index < 9; index += 1) icon.append(document.createElement("span"));
  return icon;
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
  button.append(icon, counter);
  return button;
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
