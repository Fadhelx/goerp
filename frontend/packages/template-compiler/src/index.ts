import { escape, type RenderContext, type RenderFunction, type TemplateRegistry } from "../../qweb-runtime/src/index.js";

export interface CompiledTemplate {
  name: string;
  render: RenderFunction;
}

interface TextNode {
  type: "text";
  text: string;
}

interface ElementNode {
  type: "element";
  tag: string;
  attrs: Record<string, string>;
  children: TemplateNode[];
}

type TemplateNode = TextNode | ElementNode;

interface TemplateDefinition {
  node: ElementNode;
  index: number;
}

interface TemplateNodeRef {
  node: ElementNode;
  parent: ElementNode | null;
  index: number;
}

interface RenderOptions {
  registry?: TemplateRegistry;
}

export function compileTemplate(source: string, options: RenderOptions = {}): CompiledTemplate {
  const templates = compileTemplates(source, options);
  return templates[0] ?? { name: "anonymous", render: () => "" };
}

export function compileTemplates(source: string, options: RenderOptions = {}): CompiledTemplate[] {
  const roots = parseXML(source);
  const templateNodes = resolveTemplateInheritance(collectTemplateDefinitions(roots));
  const nodes = templateNodes.length ? templateNodes : roots.filter(isElement);
  return nodes.map((node, index) => {
    const name = node.attrs["t-name"] ?? `anonymous_${index + 1}`;
    return {
      name,
      render(context: RenderContext = {}) {
        return renderTemplateRoot(node, createScope(context), options);
      }
    };
  });
}

export function registerTemplates(registry: TemplateRegistry, source: string): CompiledTemplate[] {
  const compiled = compileTemplates(source, { registry });
  for (const template of compiled) {
    registry.add(template.name, template.render);
  }
  return compiled;
}

function resolveTemplateInheritance(definitions: TemplateDefinition[]): ElementNode[] {
  const templates = new Map<string, ElementNode>();
  const outputNames: string[] = [];
  for (const definition of definitions) {
    const inherit = definition.node.attrs["t-inherit"];
    const name = definition.node.attrs["t-name"];
    if (!inherit && name) {
      templates.set(name, cloneElement(definition.node));
      outputNames.push(name);
    }
  }
  for (const definition of definitions) {
    const inherit = definition.node.attrs["t-inherit"];
    if (!inherit) continue;
    const parent = templates.get(inherit);
    if (!parent) throw new Error(`template inheritance parent not found: ${inherit}`);
    const name = definition.node.attrs["t-name"];
    const mode = definition.node.attrs["t-inherit-mode"] || (name ? "primary" : "extension");
    if (mode === "extension") {
      applyTemplatePatches(parent, definition.node);
      continue;
    }
    if (mode !== "primary") throw new Error(`unsupported t-inherit-mode: ${mode}`);
    if (!name) throw new Error(`primary template inheritance requires t-name`);
    const derived = cloneElement(parent);
    derived.attrs["t-name"] = name;
    delete derived.attrs["t-inherit"];
    delete derived.attrs["t-inherit-mode"];
    applyTemplatePatches(derived, definition.node);
    templates.set(name, derived);
    outputNames.push(name);
  }
  return outputNames.map((name) => templates.get(name)).filter((node): node is ElementNode => Boolean(node));
}

function applyTemplatePatches(target: ElementNode, patchTemplate: ElementNode): void {
  for (const patch of patchTemplate.children.filter(isElement)) {
    const position = patch.attrs.position || "inside";
    if (patch.tag === "xpath") {
      const expr = patch.attrs.expr;
      if (!expr) throw new Error("template xpath requires expr");
      const refs = findTemplateXPathRefs(target, expr);
      if (!refs.length) throw new Error(`template xpath did not match: ${expr}`);
      applyTemplatePatchRefs(target, refs, patch, position);
      continue;
    }
    if (!patch.attrs.position) continue;
    const refs = findTemplateDirectRefs(target, patch);
    if (!refs.length) throw new Error(`template locator did not match: ${patch.tag}`);
    applyTemplatePatchRefs(target, refs, patch, position);
  }
}

function applyTemplatePatchRefs(root: ElementNode, refs: TemplateNodeRef[], patch: ElementNode, position: string): void {
  switch (position) {
    case "inside":
      for (const ref of refs) ref.node.children.push(...cloneNodes(templatePatchContent(patch)));
      return;
    case "before":
      for (const ref of refs.slice().reverse()) {
        if (!ref.parent) throw new Error("cannot insert before template root");
        ref.parent.children.splice(ref.index, 0, ...cloneNodes(templatePatchContent(patch)));
      }
      return;
    case "after":
      for (const ref of refs.slice().reverse()) {
        if (!ref.parent) throw new Error("cannot insert after template root");
        ref.parent.children.splice(ref.index + 1, 0, ...cloneNodes(templatePatchContent(patch)));
      }
      return;
    case "replace":
      for (const ref of refs.slice().reverse()) {
        const content = expandTemplateZero(templatePatchContent(patch), ref.node);
        if (!ref.parent) {
          if (content.length !== 1 || !isElement(content[0])) throw new Error("template root replacement requires one element");
          root.tag = content[0].tag;
          root.attrs = { ...content[0].attrs };
          root.children = cloneNodes(content[0].children);
          continue;
        }
        ref.parent.children.splice(ref.index, 1, ...cloneNodes(content));
      }
      return;
    case "attributes":
      for (const ref of refs) applyTemplateAttributePatch(ref.node, patch);
      return;
    default:
      throw new Error(`unsupported template inheritance position: ${position}`);
  }
}

function templatePatchContent(patch: ElementNode): TemplateNode[] {
  if (patch.tag === "xpath") return patch.children;
  return patch.children;
}

function applyTemplateAttributePatch(target: ElementNode, patch: ElementNode): void {
  for (const attrNode of patch.children.filter(isElement)) {
    if (attrNode.tag !== "attribute") continue;
    const name = attrNode.attrs.name;
    if (!name) throw new Error("template attribute patch requires name");
    const current = target.attrs[name] ?? "";
    const remove = attrNode.attrs.remove;
    const add = attrNode.attrs.add;
    const separator = attrNode.attrs.separator ?? " ";
    if (remove !== undefined || add !== undefined) {
      let parts = current ? current.split(separator).filter(Boolean) : [];
      if (remove !== undefined) parts = parts.filter((part) => part !== remove);
      if (add !== undefined && !parts.includes(add)) parts.push(add);
      const next = parts.join(separator);
      if (next) target.attrs[name] = next;
      else delete target.attrs[name];
      continue;
    }
    const value = textContent(attrNode.children).trim();
    if (value) target.attrs[name] = value;
    else delete target.attrs[name];
  }
}

function expandTemplateZero(nodes: TemplateNode[], target: ElementNode): TemplateNode[] {
  const out: TemplateNode[] = [];
  for (const node of nodes) {
    if (node.type === "text" && node.text === "$0") {
      out.push(cloneElement(target));
      continue;
    }
    if (node.type === "element") {
      const clone = cloneElement(node);
      clone.children = expandTemplateZero(node.children, target);
      out.push(clone);
      continue;
    }
    out.push(cloneNode(node));
  }
  return out;
}

function findTemplateDirectRefs(root: ElementNode, locator: ElementNode): TemplateNodeRef[] {
  const refs: TemplateNodeRef[] = [];
  walkTemplateRefs(root, null, 0, (ref) => {
    if (ref.node.tag !== locator.tag) return;
    for (const [name, value] of Object.entries(locator.attrs)) {
      if (name === "position") continue;
      if (ref.node.attrs[name] !== value) return;
    }
    refs.push(ref);
  });
  return refs;
}

function findTemplateXPathRefs(root: ElementNode, expr: string): TemplateNodeRef[] {
  expr = expr.trim();
  if (expr === ".") return [{ node: root, parent: null, index: 0 }];
  if (!expr.startsWith("//")) return [];
  const segments = splitTemplateXPath(expr.slice(2));
  let refs: TemplateNodeRef[] = [];
  walkTemplateRefs(root, null, 0, (ref) => {
    if (matchesTemplateXPathSegment(ref.node, segments[0])) refs.push(ref);
  });
  for (const segment of segments.slice(1)) {
    const next: TemplateNodeRef[] = [];
    for (const ref of refs) {
      ref.node.children.forEach((child, index) => {
        if (isElement(child) && matchesTemplateXPathSegment(child, segment)) {
          next.push({ node: child, parent: ref.node, index });
        }
      });
    }
    refs = next;
  }
  return refs;
}

function splitTemplateXPath(path: string): string[] {
  const segments: string[] = [];
  let start = 0;
  let quote: string | null = null;
  let depth = 0;
  for (let index = 0; index < path.length; index += 1) {
    const char = path[index];
    if (quote) {
      if (char === quote) quote = null;
      continue;
    }
    if (char === `"` || char === `'`) {
      quote = char;
      continue;
    }
    if (char === "[") depth += 1;
    if (char === "]") depth -= 1;
    if (char === "/" && depth === 0) {
      const segment = path.slice(start, index).trim();
      if (segment) segments.push(segment);
      start = index + 1;
    }
  }
  const segment = path.slice(start).trim();
  if (segment) segments.push(segment);
  return segments;
}

function matchesTemplateXPathSegment(node: ElementNode, segment: string): boolean {
  const bracket = segment.indexOf("[");
  const tag = bracket < 0 ? segment.trim() : segment.slice(0, bracket).trim();
  if (tag && tag !== "*" && node.tag !== tag) return false;
  if (bracket < 0) return true;
  for (const predicate of templateXPathPredicates(segment.slice(bracket))) {
    if (!matchesTemplateXPathPredicate(node, predicate)) return false;
  }
  return true;
}

function templateXPathPredicates(raw: string): string[] {
  const predicates: string[] = [];
  for (let index = 0; index < raw.length; index += 1) {
    if (raw[index] !== "[") continue;
    const start = index + 1;
    let quote: string | null = null;
    let depth = 1;
    for (index += 1; index < raw.length; index += 1) {
      const char = raw[index];
      if (quote) {
        if (char === quote) quote = null;
        continue;
      }
      if (char === `"` || char === `'`) {
        quote = char;
        continue;
      }
      if (char === "[") depth += 1;
      if (char === "]") {
        depth -= 1;
        if (depth === 0) {
          const predicate = raw.slice(start, index).trim();
          if (predicate) predicates.push(predicate);
          break;
        }
      }
    }
  }
  return predicates;
}

function matchesTemplateXPathPredicate(node: ElementNode, predicate: string): boolean {
  const hasClass = /^hasclass\((.*)\)$/.exec(predicate);
  if (hasClass) {
    const tokens = new Set((node.attrs.class ?? "").split(/\s+/).filter(Boolean));
    const classNames = splitTemplateXPathArgs(hasClass[1]).map(cleanTemplateStringArg).filter(Boolean);
    return classNames.length > 0 && classNames.every((className) => tokens.has(className));
  }
  const attrEquals = /^@([^=\s]+)\s*=\s*(['"])(.*?)\2$/.exec(predicate);
  if (attrEquals) return node.attrs[attrEquals[1]] === attrEquals[3];
  const attrPresent = /^@([^=\s]+)$/.exec(predicate);
  if (attrPresent) return Object.prototype.hasOwnProperty.call(node.attrs, attrPresent[1]);
  return false;
}

function splitTemplateXPathArgs(args: string): string[] {
  const out: string[] = [];
  let quote: string | null = null;
  let start = 0;
  for (let index = 0; index < args.length; index += 1) {
    const char = args[index];
    if (quote) {
      if (char === quote) quote = null;
      continue;
    }
    if (char === `"` || char === `'`) {
      quote = char;
      continue;
    }
    if (char === ",") {
      const part = args.slice(start, index).trim();
      if (part) out.push(part);
      start = index + 1;
    }
  }
  const part = args.slice(start).trim();
  if (part) out.push(part);
  return out;
}

function cleanTemplateStringArg(value: string): string {
  value = value.trim();
  if (value.length >= 2 && (value[0] === `"` || value[0] === `'`) && value.at(-1) === value[0]) {
    return value.slice(1, -1);
  }
  return value;
}

function walkTemplateRefs(node: ElementNode, parent: ElementNode | null, index: number, visit: (ref: TemplateNodeRef) => void): void {
  visit({ node, parent, index });
  node.children.forEach((child, childIndex) => {
    if (isElement(child)) walkTemplateRefs(child, node, childIndex, visit);
  });
}

function cloneNodes(nodes: TemplateNode[]): TemplateNode[] {
  return nodes.map(cloneNode);
}

function cloneNode(node: TemplateNode): TemplateNode {
  if (node.type === "text") return { type: "text", text: node.text };
  return cloneElement(node);
}

function cloneElement(node: ElementNode): ElementNode {
  return {
    type: "element",
    tag: node.tag,
    attrs: { ...node.attrs },
    children: cloneNodes(node.children)
  };
}

function textContent(nodes: TemplateNode[]): string {
  return nodes.map((node) => (node.type === "text" ? node.text : textContent(node.children))).join("");
}

function renderNode(node: TemplateNode, scope: RenderContext, options: RenderOptions): string {
  if (node.type === "text") return node.text;
  return renderElement(node, scope, options);
}

function renderTemplateRoot(node: ElementNode, scope: RenderContext, options: RenderOptions): string {
  const attrs = { ...node.attrs };
  delete attrs["t-name"];
  const hasRootDirective = Object.keys(attrs).some((name) => name.startsWith("t-"));
  if (!hasRootDirective && node.tag === "t") return renderConditionalChildren(node.children, scope, options);
  return renderElement({ ...node, attrs }, scope, options);
}

function renderElement(node: ElementNode, scope: RenderContext, options: RenderOptions): string {
  if (!conditionPasses(node, scope)) return "";
  if ("t-foreach" in node.attrs) return renderLoop(node, scope, options);
  if ("t-set" in node.attrs) {
    scope[node.attrs["t-set"]] =
      "t-value" in node.attrs ? evalExpr(scope, node.attrs["t-value"]) : renderChildren(node.children, scope, options);
    return "";
  }
  if ("t-call" in node.attrs) {
    const childScope = createScope(scope);
    renderChildren(node.children, childScope, options);
    const name = node.attrs["t-call"];
    return options.registry?.render(name, childScope) ?? "";
  }
  if ("t-esc" in node.attrs || "t-out" in node.attrs) {
    return escape(evalExpr(scope, node.attrs["t-esc"] ?? node.attrs["t-out"]));
  }
  if ("t-raw" in node.attrs) {
    return String(evalExpr(scope, node.attrs["t-raw"]) ?? "");
  }
  if (node.tag === "t") return renderConditionalChildren(node.children, scope, options);

  const attrs = renderAttributes(node, scope);
  const body = renderConditionalChildren(node.children, scope, options);
  if (voidTags.has(node.tag)) return `<${node.tag}${attrs}>`;
  return `<${node.tag}${attrs}>${body}</${node.tag}>`;
}

function renderLoop(node: ElementNode, scope: RenderContext, options: RenderOptions): string {
  const values = evalExpr(scope, node.attrs["t-foreach"]);
  if (!Array.isArray(values)) return "";
  const alias = node.attrs["t-as"] || "item";
  return values
    .map((value, index) => {
      const childScope = createScope(scope);
      childScope[alias] = value;
      childScope[`${alias}_index`] = index;
      childScope[`${alias}_first`] = index === 0;
      childScope[`${alias}_last`] = index === values.length - 1;
      return renderElementWithoutLoop(node, childScope, options);
    })
    .join("");
}

function renderElementWithoutLoop(node: ElementNode, scope: RenderContext, options: RenderOptions): string {
  const attrs = { ...node.attrs };
  delete attrs["t-foreach"];
  delete attrs["t-as"];
  return renderElement({ ...node, attrs }, scope, options);
}

function renderChildren(nodes: TemplateNode[], scope: RenderContext, options: RenderOptions): string {
  return nodes.map((child) => renderNode(child, scope, options)).join("");
}

function renderConditionalChildren(nodes: TemplateNode[], scope: RenderContext, options: RenderOptions): string {
  let previousConditionalMatched = false;
  let previousWasConditional = false;
  let out = "";
  for (const child of nodes) {
    if (child.type === "text" && previousWasConditional && child.text.trim() === "") {
      out += child.text;
      continue;
    }
    if (child.type !== "element" || (!("t-if" in child.attrs) && !("t-elif" in child.attrs) && !("t-else" in child.attrs))) {
      previousConditionalMatched = false;
      previousWasConditional = false;
      out += renderNode(child, scope, options);
      continue;
    }
    if ("t-if" in child.attrs) {
      previousConditionalMatched = truthy(evalExpr(scope, child.attrs["t-if"]));
      previousWasConditional = true;
      if (previousConditionalMatched) out += renderElement(child, scope, options);
      continue;
    }
    if ("t-elif" in child.attrs) {
      const matched = previousWasConditional && !previousConditionalMatched && truthy(evalExpr(scope, child.attrs["t-elif"]));
      previousConditionalMatched = previousConditionalMatched || matched;
      if (matched) out += renderElement(child, scope, options);
      continue;
    }
    if ("t-else" in child.attrs) {
      const matched = previousWasConditional && !previousConditionalMatched;
      previousConditionalMatched = true;
      if (matched) out += renderElement(child, scope, options);
    }
  }
  return out;
}

function conditionPasses(node: ElementNode, scope: RenderContext): boolean {
  if ("t-if" in node.attrs) return truthy(evalExpr(scope, node.attrs["t-if"]));
  if ("t-elif" in node.attrs || "t-else" in node.attrs) return true;
  return true;
}

function renderAttributes(node: ElementNode, scope: RenderContext): string {
  const attrs: Record<string, string> = {};
  for (const [name, value] of Object.entries(node.attrs)) {
    if (name === "t-att") {
      const dynamicAttrs = evalExpr(scope, value);
      if (dynamicAttrs && typeof dynamicAttrs === "object") {
        for (const [dynamicName, dynamicValue] of Object.entries(dynamicAttrs as Record<string, unknown>)) {
          mergeAttribute(attrs, dynamicName, dynamicValue);
        }
      }
      continue;
    }
    if (name === "t-attf-class") {
      mergeAttribute(attrs, "class", interpolate(value, scope));
      continue;
    }
    if (name.startsWith("t-attf-")) {
      mergeAttribute(attrs, name.slice("t-attf-".length), interpolate(value, scope));
      continue;
    }
    if (name === "t-att-class") {
      mergeAttribute(attrs, "class", classValue(evalExpr(scope, value)));
      continue;
    }
    if (name.startsWith("t-att-")) {
      mergeAttribute(attrs, name.slice("t-att-".length), evalExpr(scope, value));
      continue;
    }
    if (directiveAttrs.has(name) || name.startsWith("t-on") || name.startsWith("t-slot")) continue;
    attrs[name] = value;
  }
  return Object.entries(attrs)
    .filter(([, value]) => value !== "")
    .map(([name, value]) => ` ${name}="${escape(value)}"`)
    .join("");
}

function mergeAttribute(attrs: Record<string, string>, name: string, value: unknown): void {
  if (value === false || value === null || value === undefined) return;
  if (value === true) {
    attrs[name] = name;
    return;
  }
  const text = String(value);
  if (!text) return;
  attrs[name] = name === "class" && attrs[name] ? `${attrs[name]} ${text}` : text;
}

function classValue(value: unknown): string {
  if (typeof value === "string") return value;
  if (Array.isArray(value)) return value.map(classValue).filter(Boolean).join(" ");
  if (value && typeof value === "object") {
    return Object.entries(value as Record<string, unknown>)
      .filter(([, enabled]) => truthy(enabled))
      .map(([name]) => name)
      .join(" ");
  }
  return "";
}

function interpolate(template: string, scope: RenderContext): string {
  return template.replaceAll(/\{\{([^}]+)}}/g, (_match, expr: string) => String(evalExpr(scope, expr.trim()) ?? ""));
}

function evalExpr(scope: RenderContext, expr: string): unknown {
  const translated = translateExpr(expr);
  if (unsafeExprPattern.test(translated)) throw new Error(`unsafe expression: ${expr}`);
  try {
    return Function("context", `with (context) { return (${translated}); }`).call(scope.this ?? scope, scope);
  } catch (error) {
    if (identifierPattern.test(expr)) return resolvePath(scope, expr);
    throw error;
  }
}

function translateExpr(expr: string): string {
  return expr
    .replaceAll(/\b(and)\b/g, "&&")
    .replaceAll(/\b(or)\b/g, "||")
    .replaceAll(/\bnot\b/g, "!")
    .replaceAll(/\blt\b/g, "<")
    .replaceAll(/\bgt\b/g, ">")
    .replaceAll(/\blte\b/g, "<=")
    .replaceAll(/\bgte\b/g, ">=");
}

function resolvePath(context: RenderContext, expr: string): unknown {
  if (expr.split(".").some((part) => unsafeNameParts.has(part))) {
    throw new Error(`unsafe expression: ${expr}`);
  }
  return expr.split(".").reduce<unknown>((value, key) => {
    if (value && typeof value === "object" && key in value) {
      return (value as Record<string, unknown>)[key];
    }
    return undefined;
  }, context);
}

function parseXML(source: string): TemplateNode[] {
  const roots: TemplateNode[] = [];
  const stack: ElementNode[] = [];
  let index = 0;
  while (index < source.length) {
    const open = source.indexOf("<", index);
    if (open < 0) {
      appendText(source.slice(index), roots, stack);
      break;
    }
    appendText(source.slice(index, open), roots, stack);
    if (source.startsWith("<!--", open)) {
      index = source.indexOf("-->", open + 4);
      index = index < 0 ? source.length : index + 3;
      continue;
    }
    if (source.startsWith("<?", open)) {
      index = source.indexOf("?>", open + 2);
      index = index < 0 ? source.length : index + 2;
      continue;
    }
    const close = findTagEnd(source, open + 1);
    if (close < 0) break;
    const raw = source.slice(open + 1, close).trim();
    index = close + 1;
    if (!raw || raw.startsWith("!")) continue;
    if (raw.startsWith("/")) {
      stack.pop();
      continue;
    }
    const selfClosing = raw.endsWith("/");
    const node = parseElement(raw.replace(/\/$/, "").trim());
    appendNode(node, roots, stack);
    if (!selfClosing && !voidTags.has(node.tag)) stack.push(node);
  }
  return roots;
}

function parseElement(raw: string): ElementNode {
  const firstSpace = raw.search(/\s/);
  const tag = firstSpace < 0 ? raw : raw.slice(0, firstSpace);
  const attrsSource = firstSpace < 0 ? "" : raw.slice(firstSpace + 1);
  return { type: "element", tag, attrs: parseAttrs(attrsSource), children: [] };
}

function parseAttrs(source: string): Record<string, string> {
  const attrs: Record<string, string> = {};
  const attrPattern = /([^\s=]+)(?:\s*=\s*("([^"]*)"|'([^']*)'))?/g;
  for (const match of source.matchAll(attrPattern)) {
    attrs[match[1]] = decodeEntities(match[3] ?? match[4] ?? "");
  }
  return attrs;
}

function appendText(text: string, roots: TemplateNode[], stack: ElementNode[]): void {
  if (!text) return;
  appendNode({ type: "text", text: decodeEntities(text) }, roots, stack);
}

function appendNode(node: TemplateNode, roots: TemplateNode[], stack: ElementNode[]): void {
  const parent = stack.at(-1);
  if (parent) parent.children.push(node);
  else roots.push(node);
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

function collectTemplateDefinitions(nodes: TemplateNode[]): TemplateDefinition[] {
  const out: TemplateDefinition[] = [];
  for (const node of nodes) {
    if (node.type !== "element") continue;
    if (node.attrs["t-name"] || node.attrs["t-inherit"]) out.push({ node, index: out.length });
    out.push(...collectTemplateDefinitions(node.children));
  }
  return out;
}

function createScope(context: RenderContext): RenderContext {
  return Object.create(context);
}

function decodeEntities(value: string): string {
  return value
    .replaceAll("&amp;", "&")
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&quot;", `"`)
    .replaceAll("&apos;", "'");
}

function isElement(node: TemplateNode): node is ElementNode {
  return node.type === "element";
}

function truthy(value: unknown): boolean {
  return Boolean(value);
}

const directiveAttrs = new Set([
  "t-name",
  "t-if",
  "t-elif",
  "t-else",
  "t-foreach",
  "t-as",
  "t-key",
  "t-set",
  "t-value",
  "t-call",
  "t-esc",
  "t-out",
  "t-raw",
  "t-inherit",
  "t-inherit-mode",
  "t-att",
  "t-att-class",
  "t-ref",
  "t-component",
  "t-props",
  "t-portal",
  "t-set-slot",
  "t-slot-scope",
  "t-slot"
]);

const voidTags = new Set(["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr"]);
const unsafeNameParts = new Set(["constructor", "prototype", "__proto__"]);
const unsafeExprPattern = /\b(?:constructor|prototype|__proto__|globalThis|window|document|Function|eval|process|require|import)\b/;
const identifierPattern = /^[a-zA-Z_$][a-zA-Z0-9_$.]*$/;
