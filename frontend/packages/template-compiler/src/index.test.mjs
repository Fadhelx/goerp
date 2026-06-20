import assert from "node:assert/strict";
import { TemplateRegistry } from "../../../dist/packages/qweb-runtime/src/index.js";
import { compileTemplate, compileTemplates, registerTemplates } from "../../../dist/packages/template-compiler/src/index.js";

const hello = compileTemplate('<t t-name="hello" t-esc="user.name"/>');
assert.equal(hello.name, "hello");
assert.equal(hello.render({ user: { name: "<Admin>" } }), "&lt;Admin&gt;");

const list = compileTemplate('<t t-name="items" t-foreach="items" t-as="item" t-esc="item.name"/>');
assert.equal(list.render({ items: [{ name: "A" }, { name: "B" }] }), "AB");

const unsafe = compileTemplate('<t t-name="bad" t-esc="constructor.constructor"/>');
assert.throws(() => unsafe.render({}), /unsafe|undefined/);

const branches = compileTemplate(`
  <templates>
    <t t-name="branch">
      <t t-if="state === 'a'">A</t>
      <t t-elif="state === 'b'">B</t>
      <t t-else="">C</t>
    </t>
  </templates>
`);
assert.equal(branches.render({ state: "a" }).trim(), "A");
assert.equal(branches.render({ state: "b" }).trim(), "B");
assert.equal(branches.render({ state: "c" }).trim(), "C");

const attrs = compileTemplate(`
  <t t-name="attrs">
    <button class="base" t-att-class="{ active: isActive, hidden: false }" t-att-data-id="record.id" t-attf-title="Hello {{record.name}}">
      <t t-esc="record.name"/>
    </button>
  </t>
`);
assert.equal(
  compact(attrs.render({ isActive: true, record: { id: 7, name: "<Admin>" } })),
  '<button class="base active" data-id="7" title="Hello &lt;Admin&gt;">&lt;Admin&gt;</button>'
);

const registry = new TemplateRegistry();
registerTemplates(
  registry,
  `
  <templates>
    <t t-name="child"><span><t t-esc="label"/></span></t>
    <t t-name="parent"><t t-call="child"><t t-set="label" t-value="user.name"/></t></t>
  </templates>
`
);
assert.equal(registry.render("parent", { user: { name: "Nora" } }), "<span>Nora</span>");

const multiple = compileTemplates(`
  <templates>
    <t t-name="one">1</t>
    <t t-name="two">2</t>
  </templates>
`);
assert.deepEqual(
  multiple.map((template) => template.name),
  ["one", "two"]
);

const inheritedExtension = compileTemplates(`
  <templates>
    <t t-name="web.FormViewDialog">
      <div class="modal">
        <button class="btn save">Save</button>
        <button class="btn saveish">Skip</button>
        <footer><button class="btn cancel">Cancel</button></footer>
      </div>
    </t>
    <t t-inherit="web.FormViewDialog" t-inherit-mode="extension">
      <xpath expr="//footer" position="inside"><button class="extra">Extra</button></xpath>
      <xpath expr="//button[hasclass('btn', 'save')]" position="attributes">
        <attribute name="class" add="primary" separator=" "/>
        <attribute name="data-remove">drop</attribute>
      </xpath>
      <xpath expr="//button[hasclass('btn', 'save')]" position="attributes"><attribute name="data-remove"/></xpath>
      <button class="btn cancel" position="after"><button class="after-cancel">After</button></button>
    </t>
  </templates>
`);
assert.deepEqual(
  inheritedExtension.map((template) => template.name),
  ["web.FormViewDialog"]
);
assert.equal(
  compact(inheritedExtension[0].render()),
  '<div class="modal"><button class="btn save primary">Save</button><button class="btn saveish">Skip</button><footer><button class="btn cancel">Cancel</button><button class="after-cancel">After</button><button class="extra">Extra</button></footer></div>'
);

const inheritedPrimary = compileTemplates(`
  <templates>
    <t t-name="web.BaseButton"><button class="btn"><span>Base</span></button></t>
    <t t-name="web.PrimaryButton" t-inherit="web.BaseButton" t-inherit-mode="primary">
      <xpath expr="//span" position="replace"><strong>$0</strong></xpath>
      <xpath expr="//button" position="attributes"><attribute name="data-kind">primary</attribute></xpath>
    </t>
  </templates>
`);
assert.deepEqual(
  inheritedPrimary.map((template) => template.name),
  ["web.BaseButton", "web.PrimaryButton"]
);
assert.equal(compact(inheritedPrimary[0].render()), '<button class="btn"><span>Base</span></button>');
assert.equal(
  compact(inheritedPrimary[1].render()),
  '<button class="btn" data-kind="primary"><strong><span>Base</span></strong></button>'
);

assert.throws(
  () =>
    compileTemplates(`
      <templates>
        <t t-inherit="missing.Template" t-inherit-mode="extension">
          <xpath expr="//div" position="inside"><span/></xpath>
        </t>
      </templates>
    `),
  /parent not found/
);

function compact(value) {
  return value.trim().replace(/>\s+/g, ">").replace(/\s+</g, "<");
}
