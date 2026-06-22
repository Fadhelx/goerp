import assert from "node:assert/strict";
import {
  parseSearchArch,
  searchItemFacet
} from "../../../../dist/packages/webclient/src/search/search_arch_parser.js";

const parsed = parseSearchArch(
  `
  <search>
    <field name="name"/>
    <field name="model_id"/>
    <field name="name"/>
    <filter name="active" string="Active" domain="[('active', '=', True)]"/>
    <filter name="archived" string="Archived" domain="[('active', '=', False)]"/>
    <filter name="created_on" string="Created On" date="create_date"/>
    <separator/>
    <filter name="my_records" string="My Records" domain="[('user_id', '=', uid)]" context="{'search_default_my_records': 1}"/>
    <filter name="group_create_date" string="Creation Date" context="{'group_by': 'create_date:month'}"/>
    <filter name="group_plain_create_date" string="Created" context="{'group_by': 'create_date'}"/>
  </search>
  `,
  {
    context: { search_default_active: 1, uid: 42 },
    fields: { create_date: { type: "datetime", string: "Created on" } },
    irFilters: [
      {
        id: 7,
        name: "My Favorite",
        domain: "[('state', '=', 'draft')]",
        context: "{'search_default_state': 1}",
        group_by: ["user_id"],
        user_id: 7,
        action_id: 9,
        is_default: true
      }
    ]
  }
);

assert.deepEqual(parsed.searchFields, ["name", "model_id"]);
assert.deepEqual(parsed.filters.map((item) => [item.name, item.label, item.group, item.isDefault]), [
  ["active", "Active", 0, true],
  ["archived", "Archived", 0, false],
  ["created_on", "Created On", 0, false],
  ["my_records", "My Records", 1, false]
]);
assert.deepEqual(parsed.filters[0].domain, [["active", "=", true]]);
assert.equal(parsed.filters[2].type, "dateFilter");
assert.equal(parsed.filters[2].dateField, "create_date");
assert.equal(parsed.filters[2].fieldType, "datetime");
assert.deepEqual(parsed.filters[3].domain, [["user_id", "=", 42]]);
assert.deepEqual(parsed.groupBys.map((item) => [item.name, item.label, item.groupBy]), [
  ["group_create_date", "Creation Date", ["create_date:month"]],
  ["group_plain_create_date", "Created", ["create_date:month"]]
]);
assert.deepEqual(parsed.favorites.map((item) => [item.id, item.label, item.groupBy, item.isDefault]), [
  ["favorite-7", "My Favorite", ["user_id"], true]
]);
assert.deepEqual(parsed.favorites.map((item) => [item.filterId, item.userId, item.actionId, item.isGlobal]), [
  [7, 7, 9, false]
]);
assert.deepEqual(parsed.defaultFacets.map((facet) => [facet.id, facet.type, facet.label]), [
  ["favorite-7", "favorite", "My Favorite"]
]);

const groupFacet = searchItemFacet(parsed.groupBys[0]);
assert.equal(groupFacet.id, "group-by-group_create_date-month");
assert.equal(groupFacet.type, "groupBy");
assert.equal(groupFacet.field, "create_date");
assert.equal(groupFacet.interval, "month");
assert.equal(groupFacet.categoryLabel, "Creation Date");
assert.deepEqual(groupFacet.valueLabels, ["Month"]);

const noFavoriteDefaults = parseSearchArch(
  `<search><filter name="active" string="Active" domain="[('active', '=', True)]" context="{'group_by': 'user_id'}"/></search>`,
  { context: { search_default_active: "1" } }
);
assert.deepEqual(noFavoriteDefaults.defaultFacets.map((facet) => [facet.id, facet.type]), [["filter-active", "filter"]]);
assert.deepEqual(noFavoriteDefaults.defaultFacets[0].domain, [["active", "=", true]]);
assert.deepEqual(noFavoriteDefaults.defaultFacets[0].groupBy, ["user_id"]);
