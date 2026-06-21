import assert from "node:assert/strict";
import {
  parseSearchArch,
  searchItemFacet
} from "../../../../dist/packages/webclient/src/search/search_arch_parser.js";

const parsed = parseSearchArch(
  `
  <search>
    <filter name="active" string="Active" domain="[('active', '=', True)]"/>
    <filter name="archived" string="Archived" domain="[('active', '=', False)]"/>
    <separator/>
    <filter name="my_records" string="My Records" domain="[('user_id', '=', uid)]" context="{'search_default_my_records': 1}"/>
    <filter name="group_create_date" string="Creation Date" context="{'group_by': 'create_date:month'}"/>
  </search>
  `,
  {
    context: { search_default_active: 1, uid: 42 },
    irFilters: [
      {
        id: 7,
        name: "My Favorite",
        domain: "[('state', '=', 'draft')]",
        context: "{'search_default_state': 1}",
        group_by: ["user_id"],
        user_ids: [2],
        is_default: true
      }
    ]
  }
);

assert.deepEqual(parsed.filters.map((item) => [item.name, item.label, item.group, item.isDefault]), [
  ["active", "Active", 0, true],
  ["archived", "Archived", 0, false],
  ["my_records", "My Records", 1, false]
]);
assert.deepEqual(parsed.filters[0].domain, [["active", "=", true]]);
assert.deepEqual(parsed.filters[2].domain, [["user_id", "=", 42]]);
assert.deepEqual(parsed.groupBys.map((item) => [item.name, item.label, item.groupBy]), [
  ["group_create_date", "Creation Date", ["create_date:month"]]
]);
assert.deepEqual(parsed.favorites.map((item) => [item.id, item.label, item.groupBy, item.isDefault]), [
  ["favorite-7", "My Favorite", ["user_id"], true]
]);
assert.deepEqual(parsed.defaultFacets.map((facet) => [facet.id, facet.type, facet.label]), [
  ["favorite-7", "favorite", "My Favorite"]
]);

const groupFacet = searchItemFacet(parsed.groupBys[0]);
assert.equal(groupFacet.type, "groupBy");
assert.equal(groupFacet.field, "create_date");
assert.equal(groupFacet.interval, "month");

const noFavoriteDefaults = parseSearchArch(
  `<search><filter name="active" string="Active" domain="[('active', '=', True)]" context="{'group_by': 'user_id'}"/></search>`,
  { context: { search_default_active: "1" } }
);
assert.deepEqual(noFavoriteDefaults.defaultFacets.map((facet) => [facet.id, facet.type]), [["filter-active", "filter"]]);
assert.deepEqual(noFavoriteDefaults.defaultFacets[0].domain, [["active", "=", true]]);
assert.deepEqual(noFavoriteDefaults.defaultFacets[0].groupBy, ["user_id"]);
