import assert from "node:assert/strict";
import {
  buildSearchState,
  createSearchModel,
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
