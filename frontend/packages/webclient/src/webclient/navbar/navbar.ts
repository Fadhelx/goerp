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
  header.className = "o_main_navbar";

  const brand = document.createElement("div");
  brand.className = "o-brand";
  const launcher = document.createElement("button");
  launcher.type = "button";
  launcher.className = "o-launcher-button";
  launcher.dataset.view = "apps";
  launcher.setAttribute("aria-label", "Apps");
  launcher.append(renderLauncherIcon());
  launcher.addEventListener("click", () => options.onOpenApps?.());
  const title = document.createElement("h1");
  title.textContent = "Odoo";
  brand.append(launcher, title);

  const nav = document.createElement("nav");
  nav.className = "o-nav";
  nav.setAttribute("aria-label", "Application");
  for (const app of options.apps ?? []) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = app.name;
    if (String(app.id) === String(options.activeAppId ?? "")) button.className = "active";
    button.addEventListener("click", () => options.onOpenApp?.(app));
    nav.append(button);
  }

  const systray = document.createElement("div");
  systray.className = "o-menu-systray o_menu_systray";
  systray.setAttribute("role", "toolbar");
  systray.setAttribute("aria-label", "Status");
  for (const item of defaultSystrayItems()) systray.append(renderSystrayItem(item));
  systray.append(renderCompanySwitcher(options.companyName ?? "My Company"));
  if (options.debug) systray.append(renderDebugItem());
  systray.append(renderUserMenu(options.userName ?? "Administrator"));

  header.append(brand, nav, systray);
  return header;
}

function renderLauncherIcon(): HTMLElement {
  const icon = document.createElement("span");
  icon.className = "o-launcher";
  icon.setAttribute("aria-hidden", "true");
  for (let index = 0; index < 9; index += 1) icon.append(document.createElement("span"));
  return icon;
}

function renderSystrayItem(item: SystrayItem): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `o-systray-item ${item.className}`;
  button.setAttribute("aria-label", item.label);
  const icon = document.createElement("span");
  icon.className = "o-systray-icon";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = item.label.slice(0, 1).toUpperCase();
  const counter = document.createElement("span");
  counter.className = "o-systray-counter";
  counter.textContent = String(item.count ?? 0);
  button.append(icon, counter);
  return button;
}

function renderCompanySwitcher(companyName: string): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_switch_company_menu o-company-switcher";
  button.setAttribute("aria-label", "Company");
  const label = document.createElement("span");
  label.textContent = companyName;
  button.append(label);
  return button;
}

function renderDebugItem(): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_debug_manager";
  button.textContent = "Debug";
  return button;
}

function renderUserMenu(userName: string): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o-systray-item o_user_menu o-user-menu-button";
  button.setAttribute("aria-label", "User menu");
  const label = document.createElement("span");
  label.textContent = userName;
  button.append(label);
  return button;
}
