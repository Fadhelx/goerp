import type { SearchDateInterval, SearchFacet } from "./search_model.js";

export type ParsedSearchItemType = "filter" | "dateFilter" | "groupBy" | "favorite";

export interface ParsedSearchItem {
  id: string;
  name: string;
  label: string;
  type: ParsedSearchItemType;
  filterId?: number;
  group?: number;
  domain?: readonly unknown[];
  context?: Record<string, unknown>;
  groupBy?: readonly string[];
  dateField?: string;
  fieldType?: string;
  defaultPeriod?: readonly string[];
  startYear?: number;
  endYear?: number;
  startMonth?: number;
  endMonth?: number;
  userId?: number;
  actionId?: number;
  embeddedActionId?: number;
  isGlobal?: boolean;
  isDefault?: boolean;
}

export interface SearchArchParseOptions {
  context?: Record<string, unknown>;
  irFilters?: readonly unknown[];
  fields?: Record<string, unknown>;
}

export interface ParsedSearchArch {
  searchFields: readonly string[];
  filters: readonly ParsedSearchItem[];
  groupBys: readonly ParsedSearchItem[];
  favorites: readonly ParsedSearchItem[];
  defaultFacets: readonly SearchFacet[];
}

interface SearchArchNode {
  tag: string;
  attrs: Record<string, string>;
}

export function parseSearchArch(arch: string, options: SearchArchParseOptions = {}): ParsedSearchArch {
  const context = options.context ?? {};
  const searchFields: string[] = [];
  const filters: ParsedSearchItem[] = [];
  const groupBys: ParsedSearchItem[] = [];
  let group = 0;
  for (const node of searchArchNodes(arch)) {
    if (node.tag === "separator") {
      group += 1;
      continue;
    }
    if (node.tag === "field") {
      const name = cleanFieldName(node.attrs.name);
      if (name && !searchFields.includes(name)) searchFields.push(name);
      continue;
    }
    if (node.tag !== "filter") continue;
    const parsedContext = parseContextAttribute(node.attrs.context);
    const groupBy = normalizeGroupByDescriptors(groupByFromContext(parsedContext), options.fields);
    const name = node.attrs.name || `filter_${filters.length + groupBys.length + 1}`;
    const label = node.attrs.string || name;
    const domain = parseDomainAttribute(node.attrs.domain, context);
    if (groupBy.length && (!domain || domain.length === 0)) {
      const [fieldName] = splitGroupByDescriptor(groupBy[0]);
      groupBys.push({
        id: `group-by-${name}`,
        name,
        label,
        type: "groupBy",
        context: parsedContext,
        groupBy,
        dateField: dateGroupByField(fieldName, options.fields),
        fieldType: fieldType(options.fields?.[fieldName]),
        isDefault: contextDefaultActive(context, name)
      });
      continue;
    }
    const dateField = cleanFieldName(node.attrs.date);
    const itemType: ParsedSearchItemType = dateField ? "dateFilter" : "filter";
    const startYear = numberAttr(node.attrs.start_year, -2);
    const endYear = numberAttr(node.attrs.end_year, 0);
    const startMonth = numberAttr(node.attrs.start_month, -2);
    const endMonth = numberAttr(node.attrs.end_month, 0);
    const defaultPeriod = dateFieldDefaultPeriod(node.attrs.default_period, context, name, defaultMonthPeriodID(startMonth, endMonth));
    filters.push({
      id: `filter-${name}`,
      name,
      label,
      type: itemType,
      group,
      domain,
      context: parsedContext,
      groupBy,
      dateField: dateField || undefined,
      fieldType: fieldType(options.fields?.[dateField]),
      defaultPeriod: dateField ? defaultPeriod : undefined,
      startYear: dateField ? startYear : undefined,
      endYear: dateField ? endYear : undefined,
      startMonth: dateField ? startMonth : undefined,
      endMonth: dateField ? endMonth : undefined,
      isDefault: contextDefaultActive(context, name)
    });
  }
  const favorites = parseIrFilters(options.irFilters ?? [], context);
  return {
    searchFields,
    filters,
    groupBys,
    favorites,
    defaultFacets: defaultFacets([...filters, ...groupBys, ...favorites])
  };
}

export function searchItemFacet(item: ParsedSearchItem): SearchFacet {
  if (item.type === "groupBy") {
    const descriptor = item.groupBy?.[0] || item.name;
    const [field, interval] = splitGroupByDescriptor(descriptor);
    return {
      id: interval ? `${item.id}-${interval}` : item.id,
      type: "groupBy",
      label: interval ? `${item.label}: ${dateIntervalLabel(interval)}` : item.label,
      categoryLabel: interval ? item.label : undefined,
      valueLabels: interval ? [dateIntervalLabel(interval)] : undefined,
      field,
      interval,
      context: item.context
    };
  }
  if (item.type === "favorite") {
    return {
      id: item.id,
      type: "favorite",
      label: item.label,
      domain: item.domain,
      context: item.context,
      groupBy: item.groupBy
    };
  }
  return {
    id: item.id,
    type: item.type === "dateFilter" ? "dateFilter" : "filter",
    label: item.label,
    field: item.dateField,
    domain: item.domain,
    context: item.context,
    groupBy: item.groupBy,
    group: item.group
  };
}

function defaultFacets(items: readonly ParsedSearchItem[]): SearchFacet[] {
  const defaultFavorite = items.find((item) => item.type === "favorite" && item.isDefault);
  if (defaultFavorite) return [searchItemFacet(defaultFavorite)];
  return items.filter((item) => item.isDefault).map(searchItemFacet);
}

function parseIrFilters(filters: readonly unknown[], context: Record<string, unknown>): ParsedSearchItem[] {
  const out: ParsedSearchItem[] = [];
  for (const [index, raw] of filters.entries()) {
    if (!isRecord(raw)) continue;
    const filterId = numberValue(raw.id);
    const userId = numberValue(raw.user_id);
    const actionId = numberValue(raw.action_id);
    const embeddedActionId = numberValue(raw.embedded_action_id);
    const name = stringValue(raw.name) || stringValue(raw.id) || `favorite_${index + 1}`;
    const parsedContext = parseContextAttribute(raw.context);
    const groupBy = normalizeGroupByDescriptors(groupByFromAny(raw.group_by ?? parsedContext.group_by), undefined);
    out.push({
      id: `favorite-${stringValue(raw.id) || name}`,
      name,
      label: name,
      type: "favorite",
      filterId,
      domain: parseDomainAttribute(raw.domain, context),
      context: parsedContext,
      groupBy,
      userId,
      actionId,
      embeddedActionId,
      isGlobal: userId === undefined,
      isDefault: raw.is_default === true || contextDefaultActive(context, name)
    });
  }
  return out;
}

function searchArchNodes(arch: string): SearchArchNode[] {
  if (!arch) return [];
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      return searchArchNodesFromElement(doc.documentElement);
    } catch {
      return searchArchNodesFromText(arch);
    }
  }
  return searchArchNodesFromText(arch);
}

function searchArchNodesFromElement(element: Element): SearchArchNode[] {
  const out: SearchArchNode[] = [];
  for (const child of Array.from(element.children)) {
    const tag = child.tagName.toLowerCase();
    if (tag === "field" || tag === "filter" || tag === "separator") {
      out.push({ tag, attrs: elementAttributes(child) });
    }
    out.push(...searchArchNodesFromElement(child));
  }
  return out;
}

function searchArchNodesFromText(arch: string): SearchArchNode[] {
  const out: SearchArchNode[] = [];
  let index = 0;
  while (index < arch.length) {
    const open = arch.indexOf("<", index);
    if (open < 0) break;
    if (arch.startsWith("<!--", open)) {
      const end = arch.indexOf("-->", open + 4);
      index = end < 0 ? arch.length : end + 3;
      continue;
    }
    const close = findTagEnd(arch, open + 1);
    if (close < 0) break;
    const token = arch.slice(open, close + 1);
    index = close + 1;
    if (/^<\//.test(token)) continue;
    const tagMatch = token.match(/^<([\w:.-]+)/);
    if (!tagMatch) continue;
    const tag = tagMatch[1].toLowerCase();
    if (tag === "field" || tag === "filter" || tag === "separator") out.push({ tag, attrs: xmlAttributes(token) });
  }
  return out;
}

function cleanFieldName(value: unknown): string {
  if (typeof value !== "string") return "";
  return value.trim();
}

function parseDomainAttribute(value: unknown, context: Record<string, unknown>): readonly unknown[] | undefined {
  if (Array.isArray(value)) return value;
  if (typeof value !== "string" || !value.trim()) return undefined;
  const parsed = parsePythonish(value, context);
  return Array.isArray(parsed) ? parsed : undefined;
}

function parseContextAttribute(value: unknown): Record<string, unknown> {
  if (isRecord(value)) return { ...value };
  if (typeof value !== "string" || !value.trim()) return {};
  const parsed = parsePythonish(value);
  return isRecord(parsed) ? parsed : {};
}

function groupByFromContext(context: Record<string, unknown>): string[] {
  return groupByFromAny(context.group_by);
}

function groupByFromAny(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item ?? "").trim()).filter(Boolean);
  if (typeof value === "string") {
    return value
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

function splitGroupByDescriptor(descriptor: string): [string, SearchFacet["interval"]] {
  const [field, interval] = descriptor.split(":");
  if (interval === "year" || interval === "quarter" || interval === "month" || interval === "week" || interval === "day") {
    return [field, interval];
  }
  return [field, undefined];
}

function normalizeGroupByDescriptors(descriptors: readonly string[], fields: Record<string, unknown> | undefined): string[] {
  return descriptors
    .map((descriptor) => {
      const [field, interval] = splitGroupByDescriptor(descriptor);
      if (!field) return "";
      if (interval) return `${field}:${interval}`;
      return dateGroupByField(field, fields) ? `${field}:month` : field;
    })
    .filter(Boolean);
}

function dateGroupByField(field: string, fields: Record<string, unknown> | undefined): string | undefined {
  if (!field || !fields) return undefined;
  const type = fieldType(fields[field]);
  return type === "date" || type === "datetime" ? field : undefined;
}

function fieldType(description: unknown): string {
  return isRecord(description) && typeof description.type === "string" ? description.type : "";
}

function dateIntervalLabel(interval: SearchDateInterval): string {
  if (interval === "year") return "Year";
  if (interval === "quarter") return "Quarter";
  if (interval === "month") return "Month";
  if (interval === "week") return "Week";
  return "Day";
}

function contextDefaultActive(context: Record<string, unknown>, name: string): boolean {
  const value = context[`search_default_${name}`];
  if (value === true || value === 1) return true;
  if (typeof value === "string") return Boolean(value.trim()) && !/^(false|0)$/i.test(value.trim());
  return false;
}

function dateFieldDefaultPeriod(value: unknown, context: Record<string, unknown>, name: string, fallbackPeriod: string): string[] {
  const contextValue = context[`search_default_${name}`];
  if (typeof contextValue === "string" && contextValue.trim() && !/^(true|1)$/i.test(contextValue.trim())) {
    return splitPeriodIDs(contextValue);
  }
  const attrPeriod = splitPeriodIDs(value);
  return attrPeriod.length ? attrPeriod : [fallbackPeriod];
}

function splitPeriodIDs(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item ?? "").trim()).filter(Boolean);
  if (typeof value !== "string") return [];
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function numberAttr(value: unknown, fallback: number): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function defaultMonthPeriodID(startMonth: number, endMonth: number): string {
  const lower = Math.min(startMonth, endMonth);
  const upper = Math.max(startMonth, endMonth);
  const offset = Math.max(lower, Math.min(upper, 0));
  if (offset === 0) return "month";
  return `month${offset > 0 ? "+" : ""}${offset}`;
}

function parsePythonish(value: string, context: Record<string, unknown> = {}): unknown {
  const jsonish = replaceContextIdentifiers(value, context)
    .trim()
    .replaceAll("&quot;", `"`)
    .replaceAll("&#34;", `"`)
    .replaceAll("&#39;", "'")
    .replaceAll(/\bTrue\b/g, "true")
    .replaceAll(/\bFalse\b/g, "false")
    .replaceAll(/\bNone\b/g, "null")
    .replaceAll("(", "[")
    .replaceAll(")", "]")
    .replaceAll("'", `"`);
  try {
    return JSON.parse(jsonish);
  } catch {
    return undefined;
  }
}

function replaceContextIdentifiers(value: string, context: Record<string, unknown>): string {
  const replacements: Record<string, unknown> = {
    uid: context.uid,
    active_id: context.active_id,
    active_ids: context.active_ids
  };
  let quote: string | null = null;
  let out = "";
  for (let index = 0; index < value.length;) {
    const char = value[index];
    if ((char === `"` || char === "'") && value[index - 1] !== "\\") {
      quote = quote === char ? null : quote ?? char;
      out += char;
      index += 1;
      continue;
    }
    if (!quote && /[A-Za-z_]/.test(char)) {
      const match = value.slice(index).match(/^[A-Za-z_][A-Za-z0-9_]*/);
      const identifier = match?.[0] ?? "";
      if (identifier && Object.hasOwn(replacements, identifier)) {
        out += JSON.stringify(replacements[identifier] ?? null);
        index += identifier.length;
        continue;
      }
    }
    out += char;
    index += 1;
  }
  return out;
}

function findTagEnd(source: string, start: number): number {
  let quote: string | null = null;
  for (let index = start; index < source.length; index += 1) {
    const char = source[index];
    if ((char === `"` || char === `'`) && source[index - 1] !== "\\") {
      quote = quote === char ? null : quote ?? char;
      continue;
    }
    if (char === ">" && !quote) return index;
  }
  return -1;
}

function elementAttributes(element: Element): Record<string, string> {
  const attrs: Record<string, string> = {};
  for (const attr of Array.from(element.attributes)) attrs[attr.name] = attr.value;
  return attrs;
}

function xmlAttributes(token: string): Record<string, string> {
  const attrs: Record<string, string> = {};
  const attrPattern = /([\w:.-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;
  for (const match of token.matchAll(attrPattern)) {
    attrs[match[1]] = xmlDecodeAttribute(match[2] ?? match[3] ?? "");
  }
  return attrs;
}

function xmlDecodeAttribute(value: string): string {
  return value
    .replaceAll("&quot;", `"`)
    .replaceAll("&apos;", "'")
    .replaceAll("&#39;", "'")
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&amp;", "&");
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function stringValue(value: unknown): string {
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}

function numberValue(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) return value;
  if (Array.isArray(value)) return numberValue(value[0]);
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    if (Number.isFinite(parsed) && parsed > 0) return parsed;
  }
  return undefined;
}
