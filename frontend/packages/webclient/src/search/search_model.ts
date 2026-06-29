export type SearchFacetType = "text" | "filter" | "dateFilter" | "groupBy" | "favorite";
export type SearchDateInterval = "year" | "quarter" | "month" | "week" | "day";

export interface SearchFacet {
  id: string;
  type: SearchFacetType;
  label: string;
  categoryLabel?: string;
  valueLabels?: readonly string[];
  field?: string;
  operator?: string;
  value?: unknown;
  domain?: readonly unknown[];
  context?: Record<string, unknown>;
  groupBy?: readonly string[];
  order?: string;
  interval?: SearchDateInterval;
  group?: string | number;
  dateFilterID?: string;
  datePeriodID?: string;
  dateDefaultYearID?: string;
  dateFieldType?: string;
  dateStartYear?: number;
  dateEndYear?: number;
  dateStartMonth?: number;
  dateEndMonth?: number;
}

export interface SearchModelState {
  query: string;
  facets: readonly SearchFacet[];
  domain: readonly unknown[];
  context: Record<string, unknown>;
  groupBy: readonly string[];
}

export interface SearchFacetDisplay {
  categoryLabel: string;
  valueLabels: readonly string[];
}

export interface SearchModel {
  readonly state: SearchModelState;
  setQuery(query: string): SearchModelState;
  addFacet(facet: Omit<SearchFacet, "id"> & { id?: string }): SearchModelState;
  removeFacet(id: string): SearchModelState;
  clear(): SearchModelState;
  toggleFacet(facet: Omit<SearchFacet, "id"> & { id?: string }): SearchModelState;
  activateFavorite(facet: Omit<SearchFacet, "id" | "type"> & { id?: string; type?: "favorite" }): SearchModelState;
}

export interface SearchModelOptions {
  query?: string;
  facets?: readonly SearchFacet[];
  searchFields?: readonly string[];
  baseDomain?: readonly unknown[];
  baseContext?: Record<string, unknown>;
}

let nextFacetID = 0;

export const SEARCH_DATE_INTERVALS: readonly { id: SearchDateInterval; label: string }[] = [
  { id: "year", label: "Year" },
  { id: "quarter", label: "Quarter" },
  { id: "month", label: "Month" },
  { id: "week", label: "Week" },
  { id: "day", label: "Day" }
];

export function createSearchModel(options: SearchModelOptions = {}): SearchModel {
  const searchFields = options.searchFields?.length ? [...options.searchFields] : ["display_name", "name"];
  const baseDomain = [...(options.baseDomain ?? [])];
  const baseContext = { ...(options.baseContext ?? {}) };
  let query = cleanQuery(options.query ?? "");
  let facets = [...(options.facets ?? [])].map(normalizeFacet);
  const build = (): SearchModelState => buildSearchState(query, facets, searchFields, baseDomain, baseContext);
  return {
    get state(): SearchModelState {
      return build();
    },
    setQuery(nextQuery: string): SearchModelState {
      query = cleanQuery(nextQuery);
      return build();
    },
    addFacet(facet: Omit<SearchFacet, "id"> & { id?: string }): SearchModelState {
      const normalized = normalizeFacet(facet);
      facets = facets.some((item) => item.id === normalized.id) ? facets : [...facets, normalized];
      return build();
    },
    removeFacet(id: string): SearchModelState {
      facets = facets.filter((facet) => facet.id !== id);
      return build();
    },
    clear(): SearchModelState {
      query = "";
      facets = [];
      return build();
    },
    toggleFacet(facet: Omit<SearchFacet, "id"> & { id?: string }): SearchModelState {
      const normalized = normalizeFacet(facet);
      facets = facets.some((item) => item.id === normalized.id)
        ? facets.filter((item) => item.id !== normalized.id)
        : [...facets, normalized];
      return build();
    },
    activateFavorite(facet: Omit<SearchFacet, "id" | "type"> & { id?: string; type?: "favorite" }): SearchModelState {
      query = "";
      facets = [normalizeFacet({ ...facet, type: "favorite" })];
      return build();
    }
  };
}

export function buildSearchState(
  query: string,
  facets: readonly SearchFacet[],
  searchFields: readonly string[] = ["display_name", "name"],
  baseDomain: readonly unknown[] = [],
  baseContext: Record<string, unknown> = {}
): SearchModelState {
  const domain: unknown[] = [...baseDomain];
  const context: Record<string, unknown> = { ...baseContext };
  const groupBy: string[] = [];
  const groupedFacetDomains = new Map<string | number, unknown[][]>();
  const dateFilterGroups = new Map<string, { group?: string | number; facets: SearchFacet[] }>();
  const cleanedQuery = cleanQuery(query);
  if (cleanedQuery) {
    domain.push(queryDomain(searchFields, cleanedQuery));
  }
  for (const facet of facets) {
    if (isGeneratedDateFilterFacet(facet)) {
      const id = facet.dateFilterID || facet.id;
      const existing = dateFilterGroups.get(id) ?? { group: facet.group, facets: [] };
      existing.facets.push(facet);
      if (existing.group === undefined && facet.group !== undefined) existing.group = facet.group;
      dateFilterGroups.set(id, existing);
    } else if (facet.domain) {
      const facetDomain = [...facet.domain];
      if ((facet.type === "filter" || facet.type === "dateFilter") && facet.group !== undefined) {
        groupedFacetDomains.set(facet.group, [...(groupedFacetDomains.get(facet.group) ?? []), facetDomain]);
      } else {
        domain.push(...facetDomain);
      }
    }
    if (facet.context) Object.assign(context, facet.context);
    if (facet.type === "text" && facet.field) {
      domain.push([facet.field, facet.operator || "ilike", facet.value ?? facet.label]);
    }
    for (const descriptor of facetGroupByDescriptors(facet)) {
      if (descriptor && !groupBy.includes(descriptor)) groupBy.push(descriptor);
    }
  }
  for (const dateFilter of dateFilterGroups.values()) {
    const dateDomain = generatedDateFilterDomain(dateFilter.facets);
    if (!dateDomain) continue;
    if (dateFilter.group !== undefined) {
      groupedFacetDomains.set(dateFilter.group, [...(groupedFacetDomains.get(dateFilter.group) ?? []), [dateDomain]]);
    } else {
      domain.push(...dateDomain);
    }
  }
  for (const groupDomains of groupedFacetDomains.values()) {
    domain.push(...combineGroupedFacetDomains(groupDomains));
  }
  return {
    query: cleanedQuery,
    facets: facets.map((facet) => ({ ...facet })),
    domain,
    context,
    groupBy
  };
}

export function searchFacetLabel(facet: SearchFacet): string {
  return searchFacetDisplay(facet).valueLabels.join(" or ");
}

export function searchFacetDisplay(facet: SearchFacet): SearchFacetDisplay {
  const valueLabels = normalizedValueLabels(facet);
  return {
    categoryLabel: cleanText(facet.categoryLabel) || defaultFacetCategoryLabel(facet),
    valueLabels: valueLabels.length ? valueLabels : [facet.label || facet.field || String(facet.value ?? "")]
  };
}

export function createDateGroupByFacet(
  field: string,
  label: string,
  interval: SearchDateInterval = "month",
  id = `group-by-${field}-${interval}`
): SearchFacet {
  return { id, type: "groupBy", label, field, interval };
}

export function createDateRangeFacet(
  field: string,
  label: string,
  start: string,
  end: string,
  id = `date-filter-${field}-${start}-${end}`
): SearchFacet {
  return {
    id,
    type: "dateFilter",
    label,
    field,
    domain: [
      [field, ">=", start],
      [field, "<", end]
    ]
  };
}

export function groupByDescriptor(field: string, interval?: SearchDateInterval): string {
  const cleanField = String(field ?? "").trim();
  if (!cleanField) return "";
  return interval ? `${cleanField}:${interval}` : cleanField;
}

function normalizeFacet(facet: Omit<SearchFacet, "id"> & { id?: string }): SearchFacet {
  const categoryLabel = cleanText(facet.categoryLabel);
  const valueLabels = cleanStringList(facet.valueLabels);
  return {
    ...facet,
    id: facet.id || `facet-${++nextFacetID}`,
    label: cleanText(facet.label) || facet.field || String(facet.value ?? ""),
    categoryLabel: categoryLabel || undefined,
    valueLabels: valueLabels.length ? valueLabels : undefined
  };
}

function isGeneratedDateFilterFacet(facet: SearchFacet): boolean {
  return facet.type === "dateFilter" && Boolean(facet.dateFilterID && facet.datePeriodID && facet.field);
}

function generatedDateFilterDomain(facets: readonly SearchFacet[]): unknown[] | null {
  const first = facets.find((facet) => isGeneratedDateFilterFacet(facet));
  const field = first?.field;
  if (!field) return null;
  const reference = new Date();
  const options = generatedDatePeriodOptions(reference, first);
  const selectedIDs = new Set(facets.map((facet) => String(facet.datePeriodID ?? "")).filter(Boolean));
  const selectedYears = options.filter((option) => option.kind === "year" && selectedIDs.has(option.id));
  const selectedPeriods = options.filter((option) => option.kind !== "year" && selectedIDs.has(option.id));
  if (!selectedYears.length) {
    const defaultYearID = selectedPeriods[0]?.defaultYearID || clampPeriodID("year", 0, first.dateStartYear, first.dateEndYear);
    const defaultYear = options.find((option) => option.kind === "year" && option.id === defaultYearID);
    if (defaultYear) selectedYears.push(defaultYear);
  }
  const ranges: unknown[] = [];
  if (selectedPeriods.length) {
    for (const year of selectedYears) {
      for (const period of selectedPeriods) {
        ranges.push(datePeriodRangeDomain(field, first.dateFieldType, year.year ?? reference.getFullYear(), period));
      }
    }
  } else {
    for (const year of selectedYears) ranges.push(datePeriodRangeDomain(field, first.dateFieldType, year.year ?? reference.getFullYear(), year));
  }
  if (!ranges.length) return null;
  const periodDomain = ranges.length === 1
    ? flatPrefixExpression("&", rangeOperands(ranges[0]))
    : flatPrefixExpression("|", ranges.map((range) => flatPrefixExpression("&", rangeOperands(range))));
  if (first.domain?.length) return flatPrefixExpression("&", [periodDomain, domainFragmentExpression([...first.domain])]);
  return periodDomain;
}

interface GeneratedDatePeriodOption {
  id: string;
  kind: "month" | "quarter" | "year";
  month?: number;
  quarter?: number;
  year?: number;
  defaultYearID?: string;
}

function generatedDatePeriodOptions(reference: Date, facet: SearchFacet): GeneratedDatePeriodOption[] {
  const startMonth = numberOrDefault(facet.dateStartMonth, -2);
  const endMonth = numberOrDefault(facet.dateEndMonth, 0);
  const startYear = numberOrDefault(facet.dateStartYear, -2);
  const endYear = numberOrDefault(facet.dateEndYear, 0);
  const options: GeneratedDatePeriodOption[] = [];
  for (let offset = Math.min(startMonth, endMonth); offset <= Math.max(startMonth, endMonth); offset += 1) {
    const date = addMonths(reference, offset);
    const yearOffset = date.getFullYear() - reference.getFullYear();
    options.push({
      id: periodID("month", offset),
      kind: "month",
      month: date.getMonth(),
      defaultYearID: clampPeriodID("year", yearOffset, startYear, endYear)
    });
  }
  const defaultYearID = clampPeriodID("year", 0, startYear, endYear);
  for (const quarter of [1, 2, 3, 4]) {
    options.push({ id: `${ordinalName(quarter)}_quarter`, kind: "quarter", quarter, defaultYearID });
  }
  const currentYear = reference.getFullYear();
  for (let offset = Math.min(startYear, endYear); offset <= Math.max(startYear, endYear); offset += 1) {
    options.push({ id: periodID("year", offset), kind: "year", year: currentYear + offset });
  }
  return options;
}

function datePeriodRangeDomain(field: string, fieldType: string | undefined, selectedYear: number, option: GeneratedDatePeriodOption): unknown[] {
  let start: Date;
  let end: Date;
  if (option.kind === "month") {
    const month = option.month ?? 0;
    start = new Date(selectedYear, month, 1);
    end = new Date(selectedYear, month + 1, 0);
  } else if (option.kind === "quarter") {
    const month = ((option.quarter ?? 1) - 1) * 3;
    start = new Date(selectedYear, month, 1);
    end = new Date(selectedYear, month + 3, 0);
  } else {
    start = new Date(selectedYear, 0, 1);
    end = new Date(selectedYear, 11, 31);
  }
  return [
    [field, ">=", serializeSearchDateBoundary(start, fieldType, false)],
    [field, "<=", serializeSearchDateBoundary(end, fieldType, true)]
  ];
}

function rangeOperands(range: unknown): unknown[][] {
  if (!Array.isArray(range)) return [[range]];
  if (isDomainCondition(range)) return [range];
  return range.map((item) => Array.isArray(item) && !isDomainCondition(item) ? domainFragmentExpression(item) : [item]);
}

function domainFragmentExpression(fragment: readonly unknown[]): unknown[] {
  if (!fragment.length) return [];
  if (isDomainCondition(fragment)) return [fragment];
  if (isLogicalDomainOperator(fragment[0])) return [...fragment];
  if (fragment.length === 1) {
    const single = fragment[0];
    return Array.isArray(single) && !isDomainCondition(single) ? domainFragmentExpression(single) : [single];
  }
  return flatPrefixExpression("&", fragment.map((item) => Array.isArray(item) && !isDomainCondition(item) ? domainFragmentExpression(item) : [item]));
}

function flatPrefixExpression(operator: "&" | "|", operands: readonly (readonly unknown[])[]): unknown[] {
  const compacted = operands.filter((operand) => operand.length > 0);
  if (compacted.length <= 1) return [...(compacted[0] ?? [])];
  return [
    ...Array.from({ length: compacted.length - 1 }, () => operator),
    ...compacted.flatMap((operand) => [...operand])
  ];
}

function clampPeriodID(unit: "year", offset: number, start: unknown, end: unknown): string {
  const lower = Math.min(numberOrDefault(start, -2), numberOrDefault(end, 0));
  const upper = Math.max(numberOrDefault(start, -2), numberOrDefault(end, 0));
  return periodID(unit, Math.max(lower, Math.min(upper, offset)));
}

function numberOrDefault(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : fallback;
}

function periodID(unit: "month" | "year", offset: number): string {
  if (offset === 0) return unit;
  return `${unit}${offset > 0 ? "+" : ""}${offset}`;
}

function addMonths(reference: Date, offset: number): Date {
  return new Date(reference.getFullYear(), reference.getMonth() + offset, 1);
}

function ordinalName(value: number): string {
  if (value === 1) return "first";
  if (value === 2) return "second";
  if (value === 3) return "third";
  return "fourth";
}

function serializeSearchDateBoundary(date: Date, fieldType: string | undefined, end: boolean): string {
  const ymd = serializeYMD(date);
  if (fieldType !== "datetime") return ymd;
  return `${ymd} ${end ? "23:59:59" : "00:00:00"}`;
}

function serializeYMD(date: Date): string {
  const year = String(date.getFullYear()).padStart(4, "0");
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function facetGroupByDescriptors(facet: SearchFacet): string[] {
  const descriptors: string[] = [];
  if (Array.isArray(facet.groupBy)) {
    descriptors.push(...facet.groupBy.map((item) => String(item ?? "").trim()).filter(Boolean));
  }
  if (facet.type === "groupBy") {
    const field = facet.field || String(facet.value ?? "");
    const descriptor = groupByDescriptor(field, facet.interval);
    if (descriptor) descriptors.push(descriptor);
  }
  return descriptors;
}

function combineGroupedFacetDomains(groupDomains: readonly (readonly unknown[])[]): unknown[] {
  const compacted = groupDomains.map(domainFragmentExpression).filter((item) => item.length > 0);
  if (compacted.length === 0) return [];
  if (compacted.length === 1) return [...compacted[0]];
  return flatPrefixExpression("|", compacted);
}

function prefixCombine(operator: "&" | "|", items: readonly unknown[]): unknown[] {
  if (items.length <= 1) return [items[0] ?? []];
  return [
    ...Array.from({ length: items.length - 1 }, () => operator),
    ...items
  ];
}

function isDomainCondition(value: unknown): boolean {
  return Array.isArray(value)
    && value.length >= 3
    && typeof value[0] === "string"
    && !["&", "|", "!"].includes(value[0])
    && typeof value[1] === "string";
}

function isLogicalDomainOperator(value: unknown): boolean {
  return value === "&" || value === "|" || value === "!";
}

function cleanQuery(query: string): string {
  return String(query ?? "").trim();
}

function cleanText(value: unknown): string {
  return String(value ?? "").trim();
}

function cleanStringList(values: readonly unknown[] | undefined): string[] {
  return [...(values ?? [])].map(cleanText).filter(Boolean);
}

function normalizedValueLabels(facet: SearchFacet): string[] {
  return cleanStringList(facet.valueLabels);
}

function defaultFacetCategoryLabel(facet: SearchFacet): string {
  if (facet.type === "groupBy") return "Group By";
  if (facet.type === "favorite") return "Favorite";
  if (facet.type === "text") return facet.field || "Search";
  return "Filter";
}

function queryDomain(searchFields: readonly string[], query: string): unknown {
  const fields = searchFields.filter(Boolean);
  if (fields.length <= 1) return [fields[0] ?? "display_name", "ilike", query];
  const domain: unknown[] = [];
  for (let index = 1; index < fields.length; index += 1) domain.push("|");
  for (const field of fields) domain.push([field, "ilike", query]);
  return domain;
}
