import assert from "node:assert/strict";
import { TemplateRegistry, escape } from "../../../dist/packages/qweb-runtime/src/index.js";

const registry = new TemplateRegistry();
registry.add("hello", (context) => `Hello ${escape(context.name)}`);

assert.equal(registry.render("hello", { name: "<Admin>" }), "Hello &lt;Admin&gt;");
assert.throws(() => registry.render("missing"), /template not found/);
