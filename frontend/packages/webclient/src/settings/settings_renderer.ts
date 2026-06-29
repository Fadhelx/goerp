export interface SettingsRendererInput {
  arch?: string;
  fields?: Record<string, unknown>;
  values?: Record<string, unknown>;
  activeApp?: string;
  search?: string;
  showSearchPanel?: boolean;
}

export interface SettingsRendererCallbacks {
  onAppSelect?: (app: SettingsApp) => void;
  onFieldChange?: (name: string, value: unknown) => void;
}

export interface SettingsRendererState {
  apps: SettingsApp[];
  activeAppId?: string;
  fields: Record<string, unknown>;
  values: Record<string, unknown>;
  search: string;
}

export interface SettingsApp {
  id: string;
  name?: string;
  label: string;
  attrs: Record<string, string>;
  blocks: SettingsBlock[];
}

export interface SettingsBlock {
  id: string;
  title: string;
  attrs: Record<string, string>;
  settings: SettingsSetting[];
}

export interface SettingsSetting {
  id: string;
  label: string;
  help: string;
  attrs: Record<string, string>;
  fields: SettingsField[];
}

export interface SettingsField {
  name: string;
  label: string;
  help: string;
  type: string;
  widget: string;
  value: unknown;
  attrs: Record<string, string>;
  description: unknown;
  readonly: boolean;
  required: boolean;
}

interface XMLNode {
  tag: string;
  attrs: Record<string, string>;
  children: XMLNode[];
  text: string;
}

export function createSettingsRendererState(input: SettingsRendererInput): SettingsRendererState {
  const fields = { ...(input.fields ?? {}) };
  const values = { ...(input.values ?? {}) };
  const apps = parseSettingsArch(input.arch ?? "", fields, values);
  const activeAppId = activeAppID(input.activeApp, apps);
  return { apps, activeAppId, fields, values, search: cleanText(input.search) };
}

export function parseSettingsArch(
  arch: string,
  fields: Record<string, unknown> = {},
  values: Record<string, unknown> = {}
): SettingsApp[] {
  const root = parseXML(arch);
  const appNodes = findElements(root, "app");
  if (appNodes.length) {
    return appNodes
      .map((node, index) => settingsAppFromNode(node, index, fields, values))
      .filter((app): app is SettingsApp => Boolean(app));
  }
  return fallbackSettingsApp(root, fields, values);
}

export function renderSettingsView(
  input: SettingsRendererInput,
  callbacks: SettingsRendererCallbacks = {}
): HTMLElement {
  const state = createSettingsRendererState(input);
  const showSearchPanel = input.showSearchPanel !== false;
  const root = document.createElement("section");
  root.className = [
    "o_settings_container",
    "o_form_view",
    "gorp-settings-parity",
    showSearchPanel ? "gorp-settings-with-search" : "gorp-settings-embedded",
    settingsMobileViewport() ? "gorp-settings-mobile" : ""
  ].filter(Boolean).join(" ");
  root.dataset.search = state.search;
  let activeAppId = state.activeAppId;
  if (activeAppId) root.dataset.activeApp = activeAppId;

  const toolbar = showSearchPanel ? document.createElement("div") : null;
  let search: HTMLInputElement | null = null;
  if (toolbar) {
    toolbar.className = "o_settings_search_panel";
    const searchWrapper = document.createElement("div");
    searchWrapper.className = "o_settings_search_wrapper";
    searchWrapper.setAttribute("role", "search");
    const searchIcon = document.createElement("span");
    searchIcon.className = "o_settings_search_icon";
    searchIcon.setAttribute("aria-hidden", "true");
    search = document.createElement("input");
    search.type = "search";
    search.className = "o_settings_search o_input";
    search.placeholder = "Search...";
    search.setAttribute("aria-label", "Search settings");
    search.value = state.search;
    const searchMenu = document.createElement("button");
    searchMenu.type = "button";
    searchMenu.className = "o_settings_search_dropdown";
    searchMenu.setAttribute("aria-label", "Search options");
    searchMenu.setAttribute("aria-expanded", "false");
    searchWrapper.append(searchIcon, search, searchMenu);
    toolbar.append(searchWrapper);
  }

  const sidebar = document.createElement("nav");
  sidebar.className = "o_settings_sidebar";
  sidebar.setAttribute("aria-label", "Settings applications");

  const content = document.createElement("div");
  content.className = "o_setting_container";
  const tabs = new Map<string, HTMLElement>();
  const articles = new Map<string, HTMLElement>();

  for (const app of state.apps) {
    const active = app.id === state.activeAppId;
    const tab = renderAppTab(app, active, () => selectSettingsApp(app, true));
    const article = renderApp(app, active, callbacks, root, state.search);
    tabs.set(app.id, tab);
    articles.set(app.id, article);
    sidebar.append(tab);
    content.append(article);
  }

  const empty = document.createElement("p");
  empty.className = "o_settings_no_result o_view_nocontent";
  empty.hidden = true;
  empty.textContent = "No settings found.";
  content.append(empty);

  const applySearch = () => {
    const query = cleanText(search?.value ?? state.search).toLowerCase();
    root.dataset.search = query;
    let visibleSettingCount = 0;
    for (const [appId, article] of articles) {
      const appActive = appId === activeAppId;
      article.hidden = !appActive;
      for (const block of findByClass(article, "o_settings_block")) {
        let blockVisible = false;
        for (const setting of findByClass(block, "o_setting_box")) {
          const text = (setting.dataset.searchText ?? textContent(setting)).toLowerCase();
          const visible = appActive && (!query || text.includes(query));
          setting.hidden = !visible;
          blockVisible ||= visible;
          if (visible) visibleSettingCount += 1;
        }
        block.hidden = !blockVisible;
      }
    }
    empty.hidden = visibleSettingCount > 0;
  };
  function selectSettingsApp(app: SettingsApp, emit: boolean): void {
    activeAppId = app.id;
    root.dataset.activeApp = app.id;
    for (const [id, tab] of tabs) {
      const active = id === app.id;
      tab.className = active ? "o_settings_tab active" : "o_settings_tab";
      tab.setAttribute("aria-pressed", active ? "true" : "false");
      tab.setAttribute("style", settingsTabInlineStyle(active));
    }
    applySearch();
    if (emit) callbacks.onAppSelect?.(app);
  }
  search?.addEventListener("input", applySearch);
  root.append(settingsParityStyleElement());
  if (toolbar) root.append(toolbar);
  root.append(sidebar, content);
  scheduleMobileSettingsChrome(root, sidebar, content);
  applySearch();
  return root;
}

function scheduleMobileSettingsChrome(root: HTMLElement, sidebar: HTMLElement, content: HTMLElement): void {
  applyMobileSettingsChrome(root, sidebar, content);
  globalThis.requestAnimationFrame?.(() => applyMobileSettingsChrome(root, sidebar, content));
  setTimeout(() => applyMobileSettingsChrome(root, sidebar, content), 0);
}

function applyMobileSettingsChrome(root: HTMLElement, sidebar: HTMLElement, content: HTMLElement): void {
  const viewport = (globalThis as typeof globalThis & { visualViewport?: { width?: number } }).visualViewport;
  const rootWidth = root.getBoundingClientRect?.().width || 0;
  const width = rootWidth || viewport?.width || globalThis.innerWidth || document.documentElement?.clientWidth || 0;
  const height = globalThis.innerHeight || document.documentElement?.clientHeight || 0;
  if (width > 575 && height > 860) return;
  root.setAttribute("style", "display:block;min-height:0;background:#262a36;color:#e4e4e4;");
  sidebar.setAttribute("style", "display:flex;order:-1;gap:0;width:100%;height:40px;max-height:40px;padding:0;overflow-x:auto;overflow-y:hidden;border-right:0;border-bottom:1px solid #3a3f4e;background:#20232d;");
  content.setAttribute("style", "padding:0 0 32px;overflow:visible;background:#262a36;color:#e4e4e4;");
  for (const tab of Array.from(sidebar.children)) {
    (tab as HTMLElement).setAttribute("style", "flex:0 0 auto;width:auto;min-height:40px;padding:0 16px;border-left:0;border-bottom:0;white-space:nowrap;");
  }
}

function settingsParityStyleElement(): HTMLElement {
  const style = document.createElement("style");
  style.dataset.settingsParity = "odoo19-dark";
  style.textContent = `
    .gorp-settings-parity { display: grid !important; grid-template-columns: 182px minmax(0, 1fr) !important; grid-template-rows: auto minmax(0, 1fr) !important; min-height: calc(100vh - 102px) !important; background: #262a36 !important; color: #e4e4e4 !important; }
    .gorp-settings-parity.gorp-settings-embedded { grid-template-rows: minmax(0, 1fr) !important; }
    .gorp-settings-parity .o_settings_search_panel { grid-column: 1 / -1; grid-row: 1; min-height: 44px; padding: 8px 16px; background: #262a36; border-bottom: 1px solid #3a3f4e; }
    .gorp-settings-parity .o_settings_search_wrapper { display: flex; align-items: center; max-width: 420px; min-height: 30px; border: 1px solid #00a09d; border-radius: 3px; background: #20232d; color: #e4e4e4; }
    .gorp-settings-parity .o_settings_search_icon::before { content: "⌕"; display: inline-block; width: 30px; color: #d7d9e0; text-align: center; font-size: 18px; line-height: 28px; }
    .gorp-settings-parity .o_settings_search_dropdown::before { content: "▾"; }
    .gorp-settings-parity .o_settings_search { min-height: 30px; border: 0; background: transparent; color: #f4f5f7; }
    .gorp-settings-parity .o_settings_sidebar { grid-column: 1 !important; grid-row: 2 !important; min-width: 0 !important; width: auto !important; padding: 0 !important; background: #1d2029 !important; border-right: 1px solid #3a3f4e !important; color: #e4e4e4 !important; }
    .gorp-settings-parity.gorp-settings-embedded .o_settings_sidebar { grid-row: 1 !important; }
    .gorp-settings-parity .o_settings_tab { display: flex !important; align-items: center !important; gap: 10px !important; width: 100% !important; min-height: 42px !important; padding: 0 16px !important; border: 0 !important; border-left: 3px solid transparent !important; background: transparent !important; color: #f4f5f7 !important; text-align: left !important; font-size: 14px !important; line-height: 20px !important; font-weight: 700 !important; }
    .gorp-settings-parity .o_settings_tab_icon { flex: 0 0 18px; width: 18px; height: 18px; border-radius: 4px; background: conic-gradient(from 0deg, #c060a1 0 25%, #35a6d9 0 50%, #ef5350 0 75%, #2fc6bd 0 100%); box-shadow: inset 0 0 0 4px #263445; }
    .gorp-settings-parity .o_settings_tab_label { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .gorp-settings-parity .o_settings_tab.active { border-left-color: #00a09d !important; background: #163b3f !important; color: #fff !important; }
    .gorp-settings-parity .o_setting_container { grid-column: 2 !important; grid-row: 2 !important; min-width: 0 !important; padding: 0 0 48px !important; overflow: auto !important; background: #262a36 !important; color: #e4e4e4 !important; }
    .gorp-settings-parity.gorp-settings-embedded .o_setting_container { grid-row: 1 !important; }
    .gorp-settings-parity .app_settings_block { max-width: none !important; margin: 0 !important; color: #e4e4e4 !important; }
    .gorp-settings-parity .o_settings_app_title { display: none !important; }
    .gorp-settings-parity .o_settings_block { margin: 0 !important; border-top: 0 !important; }
    .gorp-settings-parity .o_settings_block_title { margin: 0; padding: 12px 32px; background: #262a36 !important; color: #f4f5f7; font-size: 14px; line-height: 20px; font-weight: 700; }
    .gorp-settings-parity .o_setting_grid { display: grid !important; grid-template-columns: repeat(2, minmax(0, 1fr)) !important; gap: 0 !important; padding: 32px 72px 34px !important; }
    .gorp-settings-parity .o_setting_box { display: grid !important; grid-template-columns: 30px minmax(0, 1fr) !important; gap: 12px !important; min-height: 56px !important; padding: 0 24px 10px 0 !important; color: #e4e4e4 !important; border-left: 1px solid #3a3f4e !important; border-top: 0 !important; }
    .gorp-settings-parity .o_setting_left_pane { display: flex; justify-content: center; padding-top: 2px; }
    .gorp-settings-parity .o_setting_right_pane { min-width: 0; color: #e4e4e4 !important; }
    .gorp-settings-parity .o_setting_field, .gorp-settings-parity .o_setting_field label { color: #e4e4e4 !important; }
    .gorp-settings-parity .o_form_label, .gorp-settings-parity .o_setting_field_label { color: #f4f5f7 !important; font-weight: 600; }
    .gorp-settings-parity .text-muted { color: #aeb4c2 !important; }
    .gorp-settings-parity .o_setting_buttons { margin-top: 7px; display: flex; flex-wrap: wrap; gap: 8px; }
    .gorp-settings-parity .o_setting_action, .gorp-settings-parity .o_setting_link { color: #8ddad8 !important; background: transparent; border: 0; padding: 0; text-align: left; font-weight: 700; }
    .gorp-settings-parity .o_setting_action::before, .gorp-settings-parity .o_setting_link::before { content: "➜"; margin-right: 6px; }
    .gorp-settings-parity .o_setting_action:hover, .gorp-settings-parity .o_setting_action:focus-visible { color: #8ddad8 !important; text-decoration: underline; }
    .gorp-settings-parity input.o_input, .gorp-settings-parity select, .gorp-settings-parity .form-select { min-height: 30px; background: #4b4d59 !important; color: #f4f5f7 !important; border: 1px solid #5f6270 !important; border-radius: 0; }
    .gorp-settings-parity input.o_input::placeholder, .gorp-settings-parity .o_settings_search::placeholder { color: #d7d9e0 !important; opacity: 1 !important; }
    .gorp-settings-parity .gorp-settings-many2one { position: relative; display: inline-flex; align-items: stretch; min-width: 180px; }
    .gorp-settings-parity .gorp-settings-many2one input { padding-right: 28px; width: 100%; }
    .gorp-settings-parity .gorp-settings-many2one-toggle { position: absolute; top: 0; right: 0; bottom: 0; width: 28px; border: 0; border-left: 1px solid #5f6270; background: transparent; color: #c7c9d1; }
    @media (max-width: 1024px) {
      .gorp-settings-parity { display: block !important; min-height: 0 !important; }
      .gorp-settings-parity .o_settings_sidebar { display: flex !important; gap: 0 !important; width: 100% !important; height: 40px !important; max-height: 40px !important; min-height: 0 !important; box-sizing: border-box !important; padding: 0 !important; overflow-x: auto !important; overflow-y: hidden !important; border-right: 0 !important; border-bottom: 1px solid #3a3f4e !important; background: #20232d !important; }
      .gorp-settings-parity .o_settings_tab { flex: 0 0 auto !important; width: auto !important; min-height: 40px !important; padding: 0 16px !important; border-left: 0 !important; border-bottom: 0 !important; white-space: nowrap !important; }
      .gorp-settings-parity .o_settings_tab.active { background: #875a7b !important; color: #fff !important; }
      .gorp-settings-parity .o_settings_tab_icon { display: none; }
      .gorp-settings-parity .o_setting_container { display: block !important; width: 100% !important; padding: 0 0 32px !important; overflow: visible !important; }
      .gorp-settings-parity .app_settings_block { margin: 0; max-width: none; }
      .gorp-settings-parity .o_settings_app_title { margin-bottom: 12px; font-size: 20px; line-height: 26px; }
      .gorp-settings-parity .o_settings_block_title { padding: 12px 16px !important; background: #262a36 !important; }
      .gorp-settings-parity .o_setting_grid { display: block !important; padding: 40px 16px 32px 40px !important; }
      .gorp-settings-parity .o_setting_box { display: grid !important; grid-template-columns: 0 minmax(0, 1fr) !important; min-height: 52px !important; padding: 0 0 18px 14px !important; border-left: 1px solid #3a3f4e !important; }
      .gorp-settings-parity .o_setting_left_pane { display: none; }
    }
    .gorp-settings-parity.gorp-settings-mobile { display: block !important; min-height: 0 !important; }
    .gorp-settings-parity.gorp-settings-mobile .o_settings_sidebar { display: flex !important; gap: 0 !important; width: 100% !important; height: 40px !important; max-height: 40px !important; min-height: 0 !important; box-sizing: border-box !important; padding: 0 !important; overflow-x: auto !important; overflow-y: hidden !important; border-right: 0 !important; border-bottom: 1px solid #3a3f4e !important; background: #20232d !important; }
    .gorp-settings-parity.gorp-settings-mobile .o_settings_tab { flex: 0 0 auto !important; width: auto !important; min-height: 40px !important; padding: 0 16px !important; border-left: 0 !important; border-bottom: 0 !important; white-space: nowrap !important; }
    .gorp-settings-parity.gorp-settings-mobile .o_settings_tab.active { background: #875a7b !important; color: #fff !important; }
    .gorp-settings-parity.gorp-settings-mobile .o_settings_tab_icon { display: none; }
    .gorp-settings-parity.gorp-settings-mobile .o_setting_container { display: block !important; width: 100% !important; padding: 0 0 32px !important; overflow: visible !important; }
    .gorp-settings-parity.gorp-settings-mobile .app_settings_block { margin: 0; max-width: none; }
    .gorp-settings-parity.gorp-settings-mobile .o_settings_app_title { margin-bottom: 12px; font-size: 20px; line-height: 26px; }
    .gorp-settings-parity.gorp-settings-mobile .o_settings_block_title { padding: 12px 16px !important; background: #262a36 !important; }
    .gorp-settings-parity.gorp-settings-mobile .o_setting_grid { display: block !important; padding: 40px 16px 32px 40px !important; }
    .gorp-settings-parity.gorp-settings-mobile .o_setting_box { display: grid !important; grid-template-columns: 0 minmax(0, 1fr) !important; min-height: 52px !important; padding: 0 0 18px 14px !important; border-left: 1px solid #3a3f4e !important; }
    .gorp-settings-parity.gorp-settings-mobile .o_setting_left_pane { display: none; }
  `;
  return style;
}

function settingsMobileViewport(): boolean {
  const runtime = globalThis as typeof globalThis & {
    innerWidth?: number;
    visualViewport?: { width?: number };
    screen?: { width?: number };
    navigator?: { maxTouchPoints?: number };
  };
  const viewportWidth = [runtime.visualViewport?.width, runtime.innerWidth, document.documentElement?.clientWidth]
    .find((value): value is number => typeof value === "number" && Number.isFinite(value) && value > 0);
  if (viewportWidth !== undefined) return viewportWidth <= 1024;
  return Boolean(runtime.navigator?.maxTouchPoints && runtime.screen?.width && runtime.screen.width <= 1024);
}

function renderAppTab(app: SettingsApp, active: boolean, onSelect: () => void): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = active ? "o_settings_tab active" : "o_settings_tab";
  button.dataset.appId = app.id;
  button.setAttribute("style", settingsTabInlineStyle(active));
  const icon = document.createElement("span");
  icon.className = "o_settings_tab_icon";
  icon.setAttribute("aria-hidden", "true");
  const label = document.createElement("span");
  label.className = "o_settings_tab_label";
  label.textContent = app.label;
  button.append(icon, label);
  button.setAttribute("aria-pressed", active ? "true" : "false");
  button.addEventListener("click", onSelect);
  return button;
}

function settingsTabInlineStyle(active: boolean): string {
  if (settingsMobileViewport()) {
    const background = active ? "#875a7b" : "transparent";
    return `display:flex !important;align-items:center !important;gap:10px !important;flex:0 0 auto !important;width:auto !important;min-height:40px !important;padding:0 16px !important;border:0 !important;background:${background} !important;color:#fff !important;text-align:left !important;font-weight:700 !important;`;
  }
  const background = active ? "#163b3f" : "transparent";
  const borderLeft = active ? "#00a09d" : "transparent";
  return `display:flex !important;align-items:center !important;gap:10px !important;width:100% !important;min-height:42px !important;padding:0 16px !important;border:0 !important;border-left:3px solid ${borderLeft} !important;background:${background} !important;color:#f4f5f7 !important;text-align:left !important;font-weight:700 !important;`;
}

function renderApp(
  app: SettingsApp,
  active: boolean,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement,
  search: string
): HTMLElement {
  const article = document.createElement("article");
  article.className = "app_settings_block";
  article.dataset.appId = app.id;
  article.dataset.string = app.label;
  article.dataset.searchText = settingsSearchText([app.label, app.name, ...app.blocks.flatMap((block) => [
    block.title,
    ...block.settings.flatMap((setting) => [setting.label, setting.help, ...setting.fields.map((field) => field.label)])
  ])]);
  article.hidden = !active;

  const heading = document.createElement("h1");
  heading.className = "o_settings_app_title";
  heading.textContent = app.label;
  article.append(heading);

  for (const block of app.blocks) {
    article.append(renderBlock(block, callbacks, eventRoot, search));
  }
  return article;
}

function renderBlock(
  block: SettingsBlock,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement,
  search: string
): HTMLElement {
  const section = document.createElement("section");
  section.className = "o_settings_block";
  section.dataset.blockId = block.id;
  section.dataset.searchText = settingsSearchText([
    block.title,
    ...block.settings.flatMap((setting) => [setting.label, setting.help, ...setting.fields.map((field) => field.label)])
  ]);

  if (block.title) {
    const title = document.createElement("h2");
    title.className = "o_settings_block_title";
    title.setAttribute("style", "background:#434756;color:#f4f5f7;");
    title.textContent = block.title;
    section.append(title);
  }

  const grid = document.createElement("div");
  grid.className = "o_setting_grid";
  grid.dataset.blockId = block.id;
  grid.dataset.settingCount = String(block.settings.length);
  for (const setting of block.settings) {
    grid.append(renderSetting(setting, callbacks, eventRoot, search));
  }
  section.append(grid);
  return section;
}

function renderSetting(
  setting: SettingsSetting,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement,
  _search: string
): HTMLElement {
  const box = document.createElement("div");
  box.className = "o_setting_box";
  box.dataset.settingId = setting.id;
  box.dataset.searchText = settingsSearchText([
    setting.label,
    setting.help,
    ...setting.fields.flatMap((field) => [field.name, field.label, field.help])
  ]);

  const left = document.createElement("div");
  left.className = "o_setting_left_pane";
  const right = document.createElement("div");
  right.className = "o_setting_right_pane";

  const primaryBoolean = setting.fields.find((field) => field.type === "boolean" && !field.readonly);
  if (primaryBoolean) {
    left.append(renderFieldControl(primaryBoolean, callbacks, eventRoot, false));
  } else {
    left.className = "o_setting_left_pane o_setting_left_pane_empty";
    left.setAttribute("aria-hidden", "true");
  }

  if (setting.label) {
    const label = document.createElement("div");
    label.className = "o_form_label";
    label.textContent = setting.label;
    right.append(label);
  }
  if (setting.help) {
    const help = document.createElement("div");
    help.className = "text-muted";
    help.setAttribute("style", "color:#aeb4c2 !important;");
    help.textContent = setting.help;
    right.append(help);
  }

  const fieldList = document.createElement("div");
  fieldList.className = "o_setting_fields";
  for (const field of setting.fields) {
    if (field === primaryBoolean) continue;
    fieldList.append(renderFieldLine(field, callbacks, eventRoot));
  }
  if (fieldList.children.length) right.append(fieldList);

  box.append(left, right);
  return box;
}

function renderFieldLine(
  field: SettingsField,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement
): HTMLElement {
  const line = document.createElement("label");
  line.className = "o_setting_field";
  line.dataset.field = field.name;

  const caption = document.createElement("span");
  caption.className = "o_setting_field_label";
  caption.textContent = field.label;
  if (field.attrs.placeholder) line.dataset.placeholder = field.attrs.placeholder;

  line.append(caption, renderFieldControl(field, callbacks, eventRoot, true));
  return line;
}

function renderFieldControl(
  field: SettingsField,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement,
  includeLabel: boolean
): HTMLElement {
  if (field.readonly) return renderReadonlyField(field);
  if (field.type === "boolean") return renderBooleanField(field, callbacks, eventRoot, includeLabel);
  if (field.type === "selection") return renderSelectionField(field, callbacks, eventRoot);
  if (field.type === "many2one") return renderMany2OneField(field, callbacks, eventRoot);
  return renderTextField(field, callbacks, eventRoot);
}

function renderReadonlyField(field: SettingsField): HTMLElement {
  const output = document.createElement("output");
  output.className = "o_field_widget o_readonly_modifier";
  output.dataset.field = field.name;
  output.textContent = formatValue(field.value);
  return output;
}

function renderBooleanField(
  field: SettingsField,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement,
  includeLabel: boolean
): HTMLElement {
  const label = document.createElement("label");
  label.className = "form-check form-switch o_field_boolean";
  label.dataset.field = field.name;

  const input = document.createElement("input");
  input.type = "checkbox";
  input.className = "form-check-input";
  input.checked = truthy(field.value);
  input.disabled = field.readonly;
  input.dataset.field = field.name;
  input.addEventListener("change", () => emitFieldChange(eventRoot, callbacks, field.name, input.checked));
  label.append(input);

  if (includeLabel) {
    const text = document.createElement("span");
    text.className = "form-check-label";
    text.textContent = field.label;
    label.append(text);
  }
  return label;
}

function renderSelectionField(
  field: SettingsField,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement
): HTMLElement {
  const select = document.createElement("select");
  select.className = "o_field_widget o_field_selection form-select";
  select.dataset.field = field.name;
  select.value = String(field.value ?? "");
  for (const [value, label] of selectionOptions(field.description)) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    if (option.value === select.value) option.selected = true;
    select.append(option);
  }
  select.addEventListener("change", () => emitFieldChange(eventRoot, callbacks, field.name, select.value));
  return select;
}

function renderMany2OneField(
  field: SettingsField,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement
): HTMLElement {
  const current = many2OneValue(field.value);
  const root = document.createElement("span");
  root.className = "o_field_widget o_field_many2one gorp-settings-many2one";
  root.dataset.field = field.name;
  if (current.id !== undefined) root.dataset.resId = String(current.id);
  const input = document.createElement("input");
  input.type = "text";
  input.className = "o_input";
  input.dataset.field = field.name;
  input.required = field.required;
  input.value = current.displayName;
  input.setAttribute("role", "combobox");
  input.setAttribute("aria-autocomplete", "list");
  input.setAttribute("aria-haspopup", "listbox");
  input.setAttribute("aria-expanded", "false");
  input.setAttribute("autocomplete", "off");
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "o_dropdown_button gorp-settings-many2one-toggle";
  toggle.dataset.field = field.name;
  toggle.setAttribute("aria-label", `Open ${field.label}`);
  toggle.setAttribute("aria-haspopup", "listbox");
  toggle.setAttribute("aria-expanded", "false");
  const emit = () => {
    const value = input.value.trim();
    if (!value) {
      delete root.dataset.resId;
      emitFieldChange(eventRoot, callbacks, field.name, false);
      return;
    }
    emitFieldChange(eventRoot, callbacks, field.name, current.id !== undefined ? [current.id, value] : value);
  };
  input.addEventListener("input", emit);
  toggle.addEventListener("click", () => {
    input.focus?.();
    input.setAttribute("aria-expanded", "true");
    toggle.setAttribute("aria-expanded", "true");
  });
  root.append(input, toggle);
  return root;
}

function renderTextField(
  field: SettingsField,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement
): HTMLElement {
  const input = document.createElement("input");
  input.className = "o_field_widget o_input";
  input.dataset.field = field.name;
  input.required = field.required;
  input.value = formatEditableValue(field.value);
  input.type = field.type === "integer" || field.type === "float" ? "number" : "text";
  if (field.attrs.placeholder) input.placeholder = field.attrs.placeholder;
  if (field.type === "float") input.step = "any";
  input.addEventListener("input", () => emitFieldChange(eventRoot, callbacks, field.name, parseFieldInput(field, input.value)));
  return input;
}

function emitFieldChange(
  eventRoot: HTMLElement,
  callbacks: SettingsRendererCallbacks,
  name: string,
  value: unknown
): void {
  callbacks.onFieldChange?.(name, value);
  if (typeof CustomEvent === "function") {
    eventRoot.dispatchEvent(new CustomEvent("settings-field-change", {
      bubbles: true,
      detail: { name, value }
    }));
  }
}

function settingsAppFromNode(
  node: XMLNode,
  index: number,
  fields: Record<string, unknown>,
  values: Record<string, unknown>
): SettingsApp | null {
  if (invisible(node.attrs, values)) return null;
  const label = labelFromAttrs(node.attrs, `Settings ${index + 1}`);
  const name = node.attrs.name || node.attrs.key || node.attrs["data-key"];
  const id = name || slug(label) || `settings-app-${index + 1}`;
  const blocks = directElements(node, "block")
    .map((block, blockIndex) => settingsBlockFromNode(block, id, blockIndex, fields, values))
    .filter((block): block is SettingsBlock => Boolean(block));
  if (!blocks.length) return null;
  return { id, name, label, attrs: { ...node.attrs }, blocks };
}

function settingsBlockFromNode(
  node: XMLNode,
  appID: string,
  index: number,
  fields: Record<string, unknown>,
  values: Record<string, unknown>
): SettingsBlock | null {
  if (invisible(node.attrs, values)) return null;
  const title = node.attrs.title || node.attrs.string || "";
  const id = node.attrs.name || node.attrs.id || `${appID}-block-${index + 1}`;
  const settings = directElements(node, "setting")
    .map((setting, settingIndex) => settingsSettingFromNode(setting, id, settingIndex, fields, values))
    .filter((setting): setting is SettingsSetting => Boolean(setting));
  if (!settings.length) return null;
  return { id, title, attrs: { ...node.attrs }, settings };
}

function settingsSettingFromNode(
  node: XMLNode,
  blockID: string,
  index: number,
  fields: Record<string, unknown>,
  values: Record<string, unknown>
): SettingsSetting | null {
  if (invisible(node.attrs, values)) return null;
  const fieldNodes = findElements(node, "field")
    .filter((fieldNode) => fieldNode.attrs.name && !invisible(fieldNode.attrs, values));
  const parsedFields = fieldNodes.map((fieldNode) => settingsFieldFromNode(fieldNode, fields, values));
  const firstField = parsedFields[0];
  const label = settingLabel(node.attrs.string || node.attrs.title || "", firstField);
  const help = node.attrs.help || collectSettingHelp(node, fieldNodes) || firstField?.help || "";
  const id = node.attrs.id || node.attrs.name || firstField?.name || `${blockID}-setting-${index + 1}`;
  if (!parsedFields.length && !label && !help) return null;
  return { id, label, help, attrs: { ...node.attrs }, fields: parsedFields };
}

function settingsFieldFromNode(
  node: XMLNode,
  fields: Record<string, unknown>,
  values: Record<string, unknown>
): SettingsField {
  const name = node.attrs.name;
  const description = fields[name];
  const type = fieldType(description, values[name]);
  return {
    name,
    label: fieldNodeLabel(node.attrs.string, description, name),
    help: node.attrs.help || fieldHelp(description),
    type,
    widget: node.attrs.widget || fieldWidget(description),
    value: values[name],
    attrs: { ...node.attrs },
    description,
    readonly: truthyAttr(node.attrs.readonly) || truthyRecordValue(description, "readonly"),
    required: truthyAttr(node.attrs.required) || truthyRecordValue(description, "required")
  };
}

function settingLabel(candidate: string, firstField: SettingsField | undefined): string {
  if (!firstField) return candidate;
  return readableFieldLabel(candidate, firstField.name) || firstField.label;
}

function fallbackSettingsApp(
  root: XMLNode,
  fields: Record<string, unknown>,
  values: Record<string, unknown>
): SettingsApp[] {
  const parsedFields = findElements(root, "field")
    .filter((node) => node.attrs.name && !invisible(node.attrs, values))
    .map((node) => settingsFieldFromNode(node, fields, values));
  const settings = parsedFields.map((field) => ({
    id: field.name,
    label: field.label,
    help: field.help,
    attrs: {},
    fields: [field]
  }));
  if (!settings.length) return [];
  return [{
    id: "general-settings",
    label: "Settings",
    attrs: {},
    blocks: [{
      id: "general",
      title: "General Settings",
      attrs: {},
      settings
    }]
  }];
}

function activeAppID(activeApp: string | undefined, apps: readonly SettingsApp[]): string | undefined {
  if (!apps.length) return undefined;
  if (activeApp && apps.some((app) => app.id === activeApp)) return activeApp;
  return apps[0].id;
}

function parseXML(arch: string): XMLNode {
  const root: XMLNode = { tag: "#root", attrs: {}, children: [], text: "" };
  const stack = [root];
  const tokenPattern = /<!--[\s\S]*?-->|<[^>]+>|[^<]+/g;
  for (const match of arch.matchAll(tokenPattern)) {
    const token = match[0];
    if (!token || token.startsWith("<!--") || token.startsWith("<?") || token.startsWith("<!")) continue;
    if (token.startsWith("</")) {
      const tag = token.slice(2, -1).trim().toLowerCase();
      while (stack.length > 1) {
        const popped = stack.pop();
        if (popped?.tag === tag) break;
      }
      continue;
    }
    if (token.startsWith("<")) {
      const element = xmlElement(token);
      if (!element) continue;
      stack[stack.length - 1].children.push(element);
      if (!/\/>$/.test(token)) stack.push(element);
      continue;
    }
    const text = xmlDecodeText(token).replace(/\s+/g, " ").trim();
    if (text) {
      stack[stack.length - 1].children.push({ tag: "#text", attrs: {}, children: [], text });
    }
  }
  return root;
}

function xmlElement(token: string): XMLNode | null {
  const match = token.match(/^<([\w:.-]+)/);
  if (!match) return null;
  return {
    tag: match[1].toLowerCase(),
    attrs: xmlAttributes(token),
    children: [],
    text: ""
  };
}

function xmlAttributes(token: string): Record<string, string> {
  const attrs: Record<string, string> = {};
  const attrPattern = /([\w:.-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;
  for (const match of token.matchAll(attrPattern)) {
    attrs[match[1]] = xmlDecodeText(match[2] ?? match[3] ?? "");
  }
  return attrs;
}

function xmlDecodeText(value: string): string {
  return value.replace(/&(?:#(\d+)|#x([0-9a-fA-F]+)|amp|lt|gt|quot|apos);/g, (match, decimal: string | undefined, hex: string | undefined) => {
    if (decimal) return String.fromCodePoint(Number.parseInt(decimal, 10));
    if (hex) return String.fromCodePoint(Number.parseInt(hex, 16));
    switch (match) {
      case "&amp;":
        return "&";
      case "&lt;":
        return "<";
      case "&gt;":
        return ">";
      case "&quot;":
        return "\"";
      case "&apos;":
        return "'";
      default:
        return match;
    }
  });
}

function findElements(node: XMLNode, tag: string): XMLNode[] {
  const out: XMLNode[] = [];
  collectElements(node, tag, out);
  return out;
}

function collectElements(node: XMLNode, tag: string, out: XMLNode[]): void {
  for (const child of node.children) {
    if (child.tag === tag) out.push(child);
    collectElements(child, tag, out);
  }
}

function directElements(node: XMLNode, tag: string): XMLNode[] {
  return node.children.filter((child) => child.tag === tag);
}

function collectSettingHelp(node: XMLNode, fieldNodes: readonly XMLNode[]): string {
  const fieldSet = new Set(fieldNodes);
  const parts: string[] = [];
  collectText(node, fieldSet, parts);
  return parts.join(" ").replace(/\s+/g, " ").trim();
}

function collectText(node: XMLNode, skipped: ReadonlySet<XMLNode>, out: string[]): void {
  if (skipped.has(node)) return;
  if (node.tag === "#text" && node.text) out.push(node.text);
  for (const child of node.children) collectText(child, skipped, out);
}

function labelFromAttrs(attrs: Record<string, string>, fallback: string): string {
  return attrs.string || attrs["data-string"] || attrs.title || attrs.name || fallback;
}

function fieldLabel(description: unknown, fallback: string): string {
  return readableFieldLabel(stringRecordValue(description, "string", "label"), fallback) || humanFieldLabel(fallback);
}

function fieldNodeLabel(candidate: string | undefined, description: unknown, fallback: string): string {
  return readableFieldLabel(candidate, fallback) || fieldLabel(description, fallback);
}

function readableFieldLabel(candidate: string | undefined, fallback: string): string {
  const label = (candidate ?? "").trim();
  if (!label || isTechnicalFieldLabel(label, fallback)) return "";
  return label;
}

function isTechnicalFieldLabel(label: string, fallback: string): boolean {
  const normalized = label.replace(/\s+/g, " ").trim();
  return normalized === fallback || /\b(?:module|group)_[a-z0-9_]+\b/i.test(normalized);
}

function humanFieldLabel(name: string): string {
  const source = name
    .replace(/^module_/, "")
    .replace(/^group_/, "")
    .replace(/^oi_workflow_/, "")
    .replace(/^oi_/, "")
    .replace(/hr_holidays/g, "time_off")
    .replace(/uom/g, "units_of_measure");
  const words = source
    .split("_")
    .map((token) => token.trim())
    .filter(Boolean)
    .map(labelToken);
  return words.join(" ") || name;
}

function labelToken(token: string): string {
  const lower = token.toLowerCase();
  const overrides: Record<string, string> = {
    ai: "AI",
    api: "API",
    crm: "CRM",
    hr: "HR",
    id: "ID",
    sms: "SMS",
    url: "URL",
    uri: "URI",
    sale: "Sales",
    expense: "Expenses",
    holiday: "Time Off",
    holidays: "Time Off",
    stock: "Inventory",
    timesheet: "Timesheets"
  };
  return overrides[lower] || lower.charAt(0).toUpperCase() + lower.slice(1);
}

function fieldHelp(description: unknown): string {
  return stringRecordValue(description, "help") || "";
}

function fieldWidget(description: unknown): string {
  return stringRecordValue(description, "widget") || "";
}

function fieldType(description: unknown, value: unknown): string {
  const type = stringRecordValue(description, "type");
  if (type) return type;
  if (typeof value === "boolean") return "boolean";
  if (typeof value === "number") return Number.isInteger(value) ? "integer" : "float";
  return "char";
}

function selectionOptions(description: unknown): Array<[string, string]> {
  if (!isRecord(description) || !Array.isArray(description.selection)) return [];
  const out: Array<[string, string]> = [];
  for (const item of description.selection) {
    if (Array.isArray(item) && item.length >= 2) {
      out.push([String(item[0]), String(item[1])]);
    } else if (isRecord(item) && item.value !== undefined) {
      out.push([String(item.value), String(item.label ?? item.string ?? item.value)]);
    }
  }
  return out;
}

function many2OneValue(value: unknown): { id?: number; displayName: string } {
  if (Array.isArray(value)) {
    const id = Number(value[0]);
    return {
      id: Number.isFinite(id) && id > 0 ? id : undefined,
      displayName: String(value[1] ?? "").trim()
    };
  }
  if (isRecord(value)) {
    const id = Number(value.id);
    return {
      id: Number.isFinite(id) && id > 0 ? id : undefined,
      displayName: String(value.display_name ?? value.name ?? "").trim()
    };
  }
  return { displayName: formatEditableValue(value) };
}

function invisible(attrs: Record<string, string>, values: Record<string, unknown>): boolean {
  const raw = attrs.invisible;
  if (raw === undefined) return false;
  const literal = booleanLiteral(raw);
  if (literal !== undefined) return literal;
  return evaluateBooleanExpression(raw, values);
}

function evaluateBooleanExpression(expression: string, values: Record<string, unknown>): boolean {
  const source = trimOuterParens(expression.trim());
  if (!source) return false;

  const orParts = splitExpression(source, "or");
  if (orParts.length > 1) return orParts.some((part) => evaluateBooleanExpression(part, values));

  const andParts = splitExpression(source, "and");
  if (andParts.length > 1) return andParts.every((part) => evaluateBooleanExpression(part, values));

  if (source.startsWith("not ")) return !evaluateBooleanExpression(source.slice(4), values);

  const comparison = source.match(/^([\w.]+)\s*(==|!=|=)\s*(.+)$/);
  if (comparison) {
    const left = recordPathValue(values, comparison[1]);
    const right = parseLiteral(comparison[3]);
    return comparison[2] === "!=" ? left !== right : left === right;
  }

  const inMatch = source.match(/^([\w.]+)\s+(not\s+in|in)\s+\[(.*)]$/);
  if (inMatch) {
    const left = recordPathValue(values, inMatch[1]);
    const items = inMatch[3].split(",").map(parseLiteral);
    const has = items.some((item) => item === left);
    return inMatch[2] === "not in" ? !has : has;
  }

  return truthy(recordPathValue(values, source));
}

function splitExpression(expression: string, operator: "and" | "or"): string[] {
  const parts: string[] = [];
  let quote = "";
  let depth = 0;
  let start = 0;
  const needle = ` ${operator} `;
  for (let index = 0; index < expression.length; index += 1) {
    const char = expression[index];
    if (quote) {
      if (char === quote && expression[index - 1] !== "\\") quote = "";
      continue;
    }
    if (char === "\"" || char === "'") {
      quote = char;
      continue;
    }
    if (char === "(" || char === "[") depth += 1;
    if (char === ")" || char === "]") depth = Math.max(0, depth - 1);
    if (depth === 0 && expression.slice(index, index + needle.length) === needle) {
      parts.push(expression.slice(start, index).trim());
      start = index + needle.length;
      index = start - 1;
    }
  }
  if (parts.length) parts.push(expression.slice(start).trim());
  return parts;
}

function trimOuterParens(value: string): string {
  let out = value;
  while (out.startsWith("(") && out.endsWith(")")) {
    out = out.slice(1, -1).trim();
  }
  return out;
}

function parseLiteral(raw: string): unknown {
  const value = raw.trim();
  const literal = booleanLiteral(value);
  if (literal !== undefined) return literal;
  if ((value.startsWith("'") && value.endsWith("'")) || (value.startsWith("\"") && value.endsWith("\""))) {
    return value.slice(1, -1);
  }
  if (/^-?\d+(?:\.\d+)?$/.test(value)) return Number(value);
  if (value === "None" || value === "null") return null;
  return value;
}

function booleanLiteral(value: string): boolean | undefined {
  const normalized = value.trim().toLowerCase();
  if (["1", "true", "yes"].includes(normalized)) return true;
  if (["0", "false", "no"].includes(normalized)) return false;
  return undefined;
}

function truthyAttr(value: string | undefined): boolean {
  return value !== undefined && (booleanLiteral(value) ?? evaluateBooleanExpression(value, {}));
}

function truthy(value: unknown): boolean {
  if (value === false || value === null || value === undefined) return false;
  if (typeof value === "number") return value !== 0 && Number.isFinite(value);
  if (typeof value === "string") return value.length > 0 && value !== "0" && value.toLowerCase() !== "false";
  if (Array.isArray(value)) return value.length > 0;
  return true;
}

function truthyRecordValue(value: unknown, key: string): boolean {
  return isRecord(value) && truthy(value[key]);
}

function recordPathValue(values: Record<string, unknown>, path: string): unknown {
  const parts = path.split(".");
  let current: unknown = values;
  for (const part of parts) {
    if (!isRecord(current)) return undefined;
    current = current[part];
  }
  return current;
}

function parseFieldInput(field: SettingsField, value: string): unknown {
  if (field.type !== "integer" && field.type !== "float") return value;
  if (!value.trim()) return false;
  const number = Number(value);
  return Number.isFinite(number) ? number : value;
}

function formatEditableValue(value: unknown): string {
  if (Array.isArray(value)) return String(value[1] ?? value[0] ?? "");
  if (isRecord(value)) return String(value.display_name ?? value.name ?? value.id ?? "");
  return formatValue(value);
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined || value === false) return "";
  if (Array.isArray(value)) return value.map(formatValue).filter(Boolean).join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function settingsSearchText(parts: readonly unknown[]): string {
  return parts.map((part) => cleanText(part)).filter(Boolean).join(" ").toLowerCase();
}

function cleanText(value: unknown): string {
  return String(value ?? "").replace(/\s+/g, " ").trim();
}

function textContent(root: HTMLElement): string {
  const own = root.textContent || "";
  return [own, ...Array.from(root.children ?? []).map((child) => textContent(child as HTMLElement))].join(" ");
}

function findByClass(root: HTMLElement, className: string, out: HTMLElement[] = []): HTMLElement[] {
  if (String(root.className).split(/\s+/).includes(className)) out.push(root);
  for (const child of Array.from(root.children ?? [])) findByClass(child as HTMLElement, className, out);
  return out;
}

function stringRecordValue(value: unknown, ...keys: string[]): string {
  if (!isRecord(value)) return "";
  for (const key of keys) {
    const item = value[key];
    if (typeof item === "string" && item.trim()) return item;
  }
  return "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function slug(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-|-$/g, "");
}
