import assert from "node:assert/strict";
import {
  buildSearchState,
  createDateGroupByFacet,
  createDateRangeFacet,
  createSearchModel,
  groupByDescriptor,
  SEARCH_DATE_INTERVALS,
  searchFacetDisplay,
  searchFacetLabel
} from "../../../../dist/packages/webclient/src/search/search_model.js";

const search = createSearchModel({
  searchFields: ["name", "email"],
  baseDomain: [["active", "=", true]],
  baseContext: { lang: "en_US" }
});

let state = search.setQuery("azure");
assert.equal(state.query, "azure");
assert.deepEqual(state.domain, [["active", "=", true], ["|", ["name", "ilike", "azure"], ["email", "ilike", "azure"]]]);

state = search.addFacet({
  id: "customers",
  type: "filter",
  label: "Customers",
  domain: [["customer_rank", ">", 0]],
  context: { search_default_customer: 1 }
});
state = search.addFacet({
  id: "salesperson",
  type: "groupBy",
  label: "Salesperson",
  field: "user_id"
});
assert.deepEqual(state.groupBy, ["user_id"]);
assert.deepEqual(state.context, { lang: "en_US", search_default_customer: 1 });
assert.equal(searchFacetLabel(state.facets[0]), "Customers");

state = search.toggleFacet({ id: "customers", type: "filter", label: "Customers" });
assert.deepEqual(state.facets.map((facet) => facet.id), ["salesperson"]);
assert.deepEqual(search.clear(), {
  query: "",
  facets: [],
  domain: [["active", "=", true]],
  context: { lang: "en_US" },
  groupBy: []
});

assert.deepEqual(
  buildSearchState("", [{ id: "late", type: "text", label: "Late", field: "state", value: "late" }]),
  {
    query: "",
    facets: [{ id: "late", type: "text", label: "Late", field: "state", value: "late" }],
    domain: [["state", "ilike", "late"]],
    context: {},
    groupBy: []
  }
);

assert.deepEqual(SEARCH_DATE_INTERVALS.map((item) => item.id), ["year", "quarter", "month", "week", "day"]);
assert.equal(groupByDescriptor("create_date", "month"), "create_date:month");
assert.deepEqual(
  searchFacetDisplay({
    id: "state",
    type: "filter",
    label: "Status",
    categoryLabel: "Stage",
    valueLabels: ["Draft", "Done"]
  }),
  { categoryLabel: "Stage", valueLabels: ["Draft", "Done"] }
);
assert.equal(
  searchFacetLabel({
    id: "state",
    type: "filter",
    label: "Status",
    categoryLabel: "Stage",
    valueLabels: ["Draft", "Done"]
  }),
  "Draft or Done"
);
assert.deepEqual(
  buildSearchState("", [createDateGroupByFacet("create_date", "Creation Date", "quarter")]).groupBy,
  ["create_date:quarter"]
);
assert.deepEqual(
  buildSearchState("", [createDateRangeFacet("create_date", "This Month", "2026-06-01", "2026-07-01")]).domain,
  [
    ["create_date", ">=", "2026-06-01"],
    ["create_date", "<", "2026-07-01"]
  ]
);

const now = new Date();
const currentMonthStart = new Date(now.getFullYear(), now.getMonth(), 1);
const currentMonthEnd = new Date(now.getFullYear(), now.getMonth() + 1, 0);
assert.deepEqual(
  buildSearchState("", [
    {
      id: "filter-date-month",
      type: "dateFilter",
      label: "Date",
      field: "date_field",
      dateFilterID: "filter-date",
      datePeriodID: "month",
      dateDefaultYearID: "year",
      dateFieldType: "date",
      dateStartYear: -2,
      dateEndYear: 0,
      dateStartMonth: -2,
      dateEndMonth: 0
    },
    {
      id: "filter-date-year",
      type: "dateFilter",
      label: "Date",
      field: "date_field",
      dateFilterID: "filter-date",
      datePeriodID: "year",
      dateFieldType: "date",
      dateStartYear: -2,
      dateEndYear: 0,
      dateStartMonth: -2,
      dateEndMonth: 0
    }
  ]).domain,
  [
    "&",
    ["date_field", ">=", ymd(currentMonthStart)],
    ["date_field", "<=", ymd(currentMonthEnd)]
  ]
);

assert.deepEqual(
  buildSearchState("", [
    {
      id: "filter-date-month",
      type: "dateFilter",
      label: "Date",
      field: "date_field",
      group: 1,
      dateFilterID: "filter-date",
      datePeriodID: "month",
      dateDefaultYearID: "year",
      dateFieldType: "date",
      dateStartYear: -2,
      dateEndYear: 0,
      dateStartMonth: -2,
      dateEndMonth: 0,
      domain: [["active", "=", true]]
    },
    {
      id: "filter-date-year",
      type: "dateFilter",
      label: "Date",
      field: "date_field",
      group: 1,
      dateFilterID: "filter-date",
      datePeriodID: "year",
      dateFieldType: "date",
      dateStartYear: -2,
      dateEndYear: 0,
      dateStartMonth: -2,
      dateEndMonth: 0
    }
  ]).domain,
  [
    "&",
    "&",
    ["date_field", ">=", ymd(currentMonthStart)],
    ["date_field", "<=", ymd(currentMonthEnd)],
    ["active", "=", true]
  ]
);

const favoriteSearch = createSearchModel({ searchFields: ["name"] });
favoriteSearch.setQuery("draft");
let favoriteState = favoriteSearch.addFacet({ id: "state", type: "filter", label: "Draft", domain: [["state", "=", "draft"]] });
assert.equal(favoriteState.query, "draft");
favoriteState = favoriteSearch.activateFavorite({
  id: "my-favorite",
  label: "My Favorite",
  domain: [["active", "=", true]],
  context: { search_default_active: 1 },
  groupBy: ["user_id", "create_date:month"]
});
assert.equal(favoriteState.query, "");
assert.deepEqual(favoriteState.facets.map((facet) => facet.id), ["my-favorite"]);
assert.deepEqual(favoriteState.domain, [["active", "=", true]]);
assert.deepEqual(favoriteState.context, { search_default_active: 1 });
assert.deepEqual(favoriteState.groupBy, ["user_id", "create_date:month"]);

assert.deepEqual(
  buildSearchState("", [
    { id: "active", type: "filter", label: "Active", group: 1, domain: [["active", "=", true]] },
    { id: "inactive", type: "filter", label: "Inactive", group: 1, domain: [["active", "=", false]] },
    { id: "customer", type: "filter", label: "Customer", group: 2, domain: [["customer_rank", ">", 0]] }
  ]).domain,
  [
    "|",
    ["active", "=", true],
    ["active", "=", false],
    ["customer_rank", ">", 0]
  ]
);

assert.deepEqual(
  buildSearchState("", [
    { id: "customer", type: "filter", label: "Customer", domain: [["customer_rank", ">", 0]], groupBy: ["user_id"] }
  ]).groupBy,
  ["user_id"]
);

const labelSearch = createSearchModel({ searchFields: ["name"] });
const labelState = labelSearch.addFacet({
  id: "stage",
  type: "filter",
  label: "Stage",
  categoryLabel: " Pipeline Stage ",
  valueLabels: [" New ", "", "Won"],
  domain: [["stage", "in", ["new", "won"]]]
});
assert.deepEqual(labelState.facets[0], {
  id: "stage",
  type: "filter",
  label: "Stage",
  categoryLabel: "Pipeline Stage",
  valueLabels: ["New", "Won"],
  domain: [["stage", "in", ["new", "won"]]]
});

function ymd(date) {
  return [
    String(date.getFullYear()).padStart(4, "0"),
    String(date.getMonth() + 1).padStart(2, "0"),
    String(date.getDate()).padStart(2, "0")
  ].join("-");
}
