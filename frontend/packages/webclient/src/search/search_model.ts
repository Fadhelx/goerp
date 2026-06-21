export type SearchFacetType = "text" | "filter" | "dateFilter" | "groupBy" | "favorite";
export type SearchDateInterval = "year" | "quarter" | "month" | "week" | "day";

export interface SearchFacet {
  id: string;
  type: SearchFacetType;
  label: string;
  field?: string;
  operator?: string;
  value?: unknown;
  domain?: readonly unknown[];
  context?: Record<string, unknown>;
  groupBy?: readonly string[];
  interval?: SearchDateInterval;
  group?: string | number;
}

export interface SearchModelState {
  query: string;
  facets: readonly SearchFacet[];
  domain: readonly unknown[];
  context: Record<string, unknown>;
  groupBy: readonly string[];
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
  const cleanedQuery = cleanQuery(query);
  if (cleanedQuery) {
    domain.push(queryDomain(searchFields, cleanedQuery));
  }
  for (const facet of facets) {
    if (facet.domain) {
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
  return facet.label || facet.field || String(facet.value ?? "");
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
  return {
    ...facet,
    id: facet.id || `facet-${++nextFacetID}`,
    label: facet.label || facet.field || String(facet.value ?? "")
  };
}

function facetGroupByDescriptors(facet: SearchFacet): string[] {
  const descriptors: string[] = [];
  if (facet.type === "favorite" && Array.isArray(facet.groupBy)) {
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
  const compacted = groupDomains.map(compactDomainFragment).filter((item) => !isEmptyDomain(item));
  if (compacted.length === 0) return [];
  if (compacted.length === 1) {
    const single = compacted[0];
    return isDomainList(single) ? [...single] : [single];
  }
  return [prefixCombine("|", compacted)];
}

function compactDomainFragment(fragment: readonly unknown[]): unknown {
  if (fragment.length <= 1) return fragment[0] ?? [];
  return prefixCombine("&", fragment);
}

function prefixCombine(operator: "&" | "|", items: readonly unknown[]): unknown[] {
  if (items.length <= 1) return [items[0] ?? []];
  return [
    ...Array.from({ length: items.length - 1 }, () => operator),
    ...items
  ];
}

function isDomainList(value: unknown): value is readonly unknown[] {
  return Array.isArray(value) && !isDomainCondition(value);
}

function isDomainCondition(value: unknown): boolean {
  return Array.isArray(value)
    && value.length >= 3
    && typeof value[0] === "string"
    && !["&", "|", "!"].includes(value[0])
    && typeof value[1] === "string";
}

function isEmptyDomain(value: unknown): boolean {
  return Array.isArray(value) && value.length === 0;
}

function cleanQuery(query: string): string {
  return String(query ?? "").trim();
}

function queryDomain(searchFields: readonly string[], query: string): unknown {
  const fields = searchFields.filter(Boolean);
  if (fields.length <= 1) return [fields[0] ?? "display_name", "ilike", query];
  const domain: unknown[] = [];
  for (let index = 1; index < fields.length; index += 1) domain.push("|");
  for (const field of fields) domain.push([field, "ilike", query]);
  return domain;
}
