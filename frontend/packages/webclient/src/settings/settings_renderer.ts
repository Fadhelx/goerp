export interface SettingsRendererInput {
  arch?: string;
  fields?: Record<string, unknown>;
  values?: Record<string, unknown>;
  activeApp?: string;
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
  return { apps, activeAppId, fields, values };
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
  const root = document.createElement("section");
  root.className = "o_settings_container o_form_view";

  const sidebar = document.createElement("nav");
  sidebar.className = "o_settings_sidebar settings_tab";
  sidebar.setAttribute("aria-label", "Settings applications");

  const content = document.createElement("div");
  content.className = "o_setting_container";

  for (const app of state.apps) {
    const active = app.id === state.activeAppId;
    sidebar.append(renderAppTab(app, active, callbacks));
    content.append(renderApp(app, active, callbacks, root));
  }

  root.append(sidebar, content);
  return root;
}

function renderAppTab(app: SettingsApp, active: boolean, callbacks: SettingsRendererCallbacks): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = active ? "o_settings_tab active" : "o_settings_tab";
  button.dataset.appId = app.id;
  button.textContent = app.label;
  button.setAttribute("aria-pressed", active ? "true" : "false");
  button.addEventListener("click", () => callbacks.onAppSelect?.(app));
  return button;
}

function renderApp(
  app: SettingsApp,
  active: boolean,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement
): HTMLElement {
  const article = document.createElement("article");
  article.className = "app_settings_block";
  article.dataset.appId = app.id;
  article.dataset.string = app.label;
  article.hidden = !active;

  const heading = document.createElement("h1");
  heading.className = "o_settings_app_title";
  heading.textContent = app.label;
  article.append(heading);

  for (const block of app.blocks) {
    article.append(renderBlock(block, callbacks, eventRoot));
  }
  return article;
}

function renderBlock(
  block: SettingsBlock,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement
): HTMLElement {
  const section = document.createElement("section");
  section.className = "o_settings_block";
  section.dataset.blockId = block.id;

  if (block.title) {
    const title = document.createElement("h2");
    title.className = "o_settings_block_title";
    title.textContent = block.title;
    section.append(title);
  }

  for (const setting of block.settings) {
    section.append(renderSetting(setting, callbacks, eventRoot));
  }
  return section;
}

function renderSetting(
  setting: SettingsSetting,
  callbacks: SettingsRendererCallbacks,
  eventRoot: HTMLElement
): HTMLElement {
  const box = document.createElement("div");
  box.className = "o_setting_box";
  box.dataset.settingId = setting.id;

  const left = document.createElement("div");
  left.className = "o_setting_left_pane";
  const right = document.createElement("div");
  right.className = "o_setting_right_pane";

  const primaryBoolean = setting.fields.find((field) => field.type === "boolean" && !field.readonly);
  if (primaryBoolean) {
    left.append(renderFieldControl(primaryBoolean, callbacks, eventRoot, false));
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
  const label = node.attrs.string || node.attrs.title || firstField?.label || "";
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
    label: node.attrs.string || fieldLabel(description, name),
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
  return stringRecordValue(description, "string", "label") || fallback;
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
