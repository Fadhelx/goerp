export type FlowchartId = number | string;

export interface FlowchartNodeInput {
  id: FlowchartId;
  label: string;
  state?: string;
  x?: number;
  y?: number;
  width?: number;
  height?: number;
  selected?: boolean;
  disabled?: boolean;
  metadata?: Record<string, unknown>;
}

export interface FlowchartTransitionInput {
  id: FlowchartId;
  from: FlowchartId;
  to: FlowchartId;
  label?: string;
  condition?: string;
  disabled?: boolean;
  metadata?: Record<string, unknown>;
}

export interface FlowchartLayoutOptions {
  nodeWidth?: number;
  nodeHeight?: number;
  spacingX?: number;
  spacingY?: number;
  columns?: number;
}

export interface FlowchartPoint {
  x: number;
  y: number;
}

export interface FlowchartNode {
  id: FlowchartId;
  label: string;
  state?: string;
  x: number;
  y: number;
  width: number;
  height: number;
  selected: boolean;
  disabled: boolean;
  metadata: Record<string, unknown>;
}

export interface FlowchartTransition {
  id: FlowchartId;
  from: FlowchartId;
  to: FlowchartId;
  label: string;
  condition?: string;
  disabled: boolean;
  points: FlowchartPoint[];
  metadata: Record<string, unknown>;
}

export interface FlowchartValidationIssue {
  code: string;
  message: string;
  id?: FlowchartId;
}

export interface FlowchartValidation {
  valid: boolean;
  issues: FlowchartValidationIssue[];
}

export interface FlowchartGraph {
  nodes: FlowchartNode[];
  transitions: FlowchartTransition[];
  validation: FlowchartValidation;
  bounds: { x: number; y: number; width: number; height: number };
}

export interface ProcessStepSelection {
  selectedNodeId: FlowchartId;
  node: FlowchartNode;
  incoming: FlowchartTransition[];
  outgoing: FlowchartTransition[];
  availableTransitions: FlowchartTransition[];
}

export function createFlowchartGraph(
  input: { nodes: readonly FlowchartNodeInput[]; transitions?: readonly FlowchartTransitionInput[] },
  options: FlowchartLayoutOptions = {}
): FlowchartGraph {
  const layout = normalizeLayout(options);
  const nodeIds = new Set<FlowchartId>();
  const transitionIds = new Set<FlowchartId>();
  const issues: FlowchartValidationIssue[] = [];

  const nodes = input.nodes.map((node, index) => {
    if (nodeIds.has(node.id)) issues.push({ code: "duplicate_node", message: `duplicate node: ${node.id}`, id: node.id });
    nodeIds.add(node.id);
    return normalizeNode(node, index, layout);
  });

  const byId = new Map(nodes.map((node) => [node.id, node]));
  const transitions = (input.transitions ?? []).map((transition) => {
    if (transitionIds.has(transition.id)) {
      issues.push({ code: "duplicate_transition", message: `duplicate transition: ${transition.id}`, id: transition.id });
    }
    transitionIds.add(transition.id);
    if (!byId.has(transition.from)) {
      issues.push({ code: "missing_source", message: `transition source not found: ${transition.from}`, id: transition.id });
    }
    if (!byId.has(transition.to)) {
      issues.push({ code: "missing_target", message: `transition target not found: ${transition.to}`, id: transition.id });
    }
    if (transition.from === transition.to) {
      issues.push({ code: "self_transition", message: `transition loops to itself: ${transition.id}`, id: transition.id });
    }
    return normalizeTransition(transition, byId);
  });

  return {
    nodes,
    transitions,
    validation: { valid: issues.length === 0, issues },
    bounds: computeBounds(nodes)
  };
}

export function createFlowchartGraphFromDSL(source: string, options: FlowchartLayoutOptions = {}): FlowchartGraph {
  const symbols = new Map<string, FlowchartNodeInput>();
  const transitions: FlowchartTransitionInput[] = [];
  const lines = source
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);

  for (const line of lines) {
    const symbol = parseFlowchartSymbol(line);
    if (!symbol) continue;
    symbols.set(String(symbol.id), symbol);
  }

  let transitionIndex = 0;
  for (const line of lines) {
    if (!line.includes("->") || line.includes("=>")) continue;
    const parts = line.split("->").map((part) => parseFlowchartConnectionPart(part.trim()));
    for (let index = 0; index < parts.length - 1; index++) {
      const from = parts[index];
      const to = parts[index + 1];
      if (!symbols.has(from.id) || !symbols.has(to.id)) continue;
      transitions.push({
        id: `${from.id}->${to.id}:${transitionIndex++}`,
        from: from.id,
        to: to.id,
        ...(from.label ? { label: from.label } : {})
      });
    }
  }

  return createFlowchartGraph({ nodes: [...symbols.values()], transitions }, options);
}

export function validateFlowchartGraph(graph: FlowchartGraph): FlowchartValidation {
  const issues: FlowchartValidationIssue[] = [...graph.validation.issues];
  if (graph.nodes.length === 0) issues.push({ code: "empty_graph", message: "graph has no nodes" });
  return { valid: issues.length === 0, issues };
}

export function selectProcessStep(graph: FlowchartGraph, selectedNodeId: FlowchartId): ProcessStepSelection {
  const node = graph.nodes.find((candidate) => candidate.id === selectedNodeId);
  if (!node) throw new Error(`process step not found: ${selectedNodeId}`);
  const incoming = graph.transitions.filter((transition) => transition.to === selectedNodeId);
  const outgoing = graph.transitions.filter((transition) => transition.from === selectedNodeId);
  return {
    selectedNodeId,
    node: { ...node, selected: true },
    incoming,
    outgoing,
    availableTransitions: outgoing.filter((transition) => !transition.disabled)
  };
}

export function setProcessStepSelection(graph: FlowchartGraph, selectedNodeId: FlowchartId): FlowchartGraph {
  return {
    ...graph,
    nodes: graph.nodes.map((node) => ({ ...node, selected: node.id === selectedNodeId }))
  };
}

function normalizeLayout(options: FlowchartLayoutOptions): Required<FlowchartLayoutOptions> {
  return {
    nodeWidth: positiveNumber(options.nodeWidth, 180),
    nodeHeight: positiveNumber(options.nodeHeight, 64),
    spacingX: positiveNumber(options.spacingX, 72),
    spacingY: positiveNumber(options.spacingY, 56),
    columns: Math.max(1, Math.floor(positiveNumber(options.columns, 3)))
  };
}

function parseFlowchartSymbol(line: string): FlowchartNodeInput | null {
  const match = line.match(/^([^=]+)=>([^:]+):\s*(.*)$/);
  if (!match) return null;
  const id = match[1].trim();
  const symbolType = match[2].trim();
  const [rawLabel, rawState] = match[3].split("|");
  const label = nonEmpty(rawLabel) ?? id;
  return {
    id,
    label,
    ...(nonEmpty(rawState) ? { state: rawState.trim() } : {}),
    metadata: { symbolType }
  };
}

function parseFlowchartConnectionPart(part: string): { id: string; label?: string } {
  const match = part.match(/^(.+?)\(([^()]*)\)$/);
  if (!match) return { id: part };
  return { id: match[1].trim(), label: match[2].trim() };
}

function normalizeNode(
  node: FlowchartNodeInput,
  index: number,
  layout: Required<FlowchartLayoutOptions>
): FlowchartNode {
  const column = index % layout.columns;
  const row = Math.floor(index / layout.columns);
  const width = positiveNumber(node.width, layout.nodeWidth);
  const height = positiveNumber(node.height, layout.nodeHeight);
  return {
    id: node.id,
    label: requireText(node.label, "flowchart node label"),
    ...(node.state ? { state: node.state } : {}),
    x: numberOrDefault(node.x, column * (layout.nodeWidth + layout.spacingX)),
    y: numberOrDefault(node.y, row * (layout.nodeHeight + layout.spacingY)),
    width,
    height,
    selected: Boolean(node.selected),
    disabled: Boolean(node.disabled),
    metadata: { ...(node.metadata ?? {}) }
  };
}

function normalizeTransition(
  transition: FlowchartTransitionInput,
  nodes: Map<FlowchartId, FlowchartNode>
): FlowchartTransition {
  const from = nodes.get(transition.from);
  const to = nodes.get(transition.to);
  return {
    id: transition.id,
    from: transition.from,
    to: transition.to,
    label: nonEmpty(transition.label) ?? "",
    ...(transition.condition ? { condition: transition.condition } : {}),
    disabled: Boolean(transition.disabled),
    points: from && to ? connectionPoints(from, to) : [],
    metadata: { ...(transition.metadata ?? {}) }
  };
}

function connectionPoints(from: FlowchartNode, to: FlowchartNode): FlowchartPoint[] {
  return [
    { x: from.x + from.width, y: from.y + from.height / 2 },
    { x: to.x, y: to.y + to.height / 2 }
  ];
}

function computeBounds(nodes: readonly FlowchartNode[]): { x: number; y: number; width: number; height: number } {
  if (nodes.length === 0) return { x: 0, y: 0, width: 0, height: 0 };
  const minX = Math.min(...nodes.map((node) => node.x));
  const minY = Math.min(...nodes.map((node) => node.y));
  const maxX = Math.max(...nodes.map((node) => node.x + node.width));
  const maxY = Math.max(...nodes.map((node) => node.y + node.height));
  return { x: minX, y: minY, width: maxX - minX, height: maxY - minY };
}

function positiveNumber(value: number | undefined, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : fallback;
}

function numberOrDefault(value: number | undefined, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function requireText(value: string, label: string): string {
  const text = nonEmpty(value);
  if (!text) throw new Error(`${label} is required`);
  return text;
}

function nonEmpty(value: unknown): string | undefined {
  const text = typeof value === "string" ? value.trim() : "";
  return text ? text : undefined;
}
