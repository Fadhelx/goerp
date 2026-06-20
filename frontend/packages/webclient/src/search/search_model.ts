export type SearchFacetType = "text" | "filter" | "groupBy" | "favorite";

export interface SearchFacet {
  id: string;
  type: SearchFacetType;
  label: string;
  field?: string;
  operator?: string;
  value?: unknown;
  domain?: readonly unknown[];
  context?: Record<string, unknown>;
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
}

export interface SearchModelOptions {
  query?: string;
  facets?: readonly SearchFacet[];
  searchFields?: readonly string[];
  baseDomain?: readonly unknown[];
  baseContext?: Record<string, unknown>;
}

let nextFacetID = 0;

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
  const cleanedQuery = cleanQuery(query);
  if (cleanedQuery) {
    domain.push(queryDomain(searchFields, cleanedQuery));
  }
  for (const facet of facets) {
    if (facet.domain) domain.push(...facet.domain);
    if (facet.context) Object.assign(context, facet.context);
    if (facet.type === "text" && facet.field) {
      domain.push([facet.field, facet.operator || "ilike", facet.value ?? facet.label]);
    }
    if (facet.type === "groupBy") {
      const field = facet.field || String(facet.value ?? "");
      if (field && !groupBy.includes(field)) groupBy.push(field);
    }
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

function normalizeFacet(facet: Omit<SearchFacet, "id"> & { id?: string }): SearchFacet {
  return {
    ...facet,
    id: facet.id || `facet-${++nextFacetID}`,
    label: facet.label || facet.field || String(facet.value ?? "")
  };
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
