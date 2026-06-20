import assert from "node:assert/strict";
import {
  createFlowchartGraph,
  createFlowchartGraphFromDSL,
  selectProcessStep,
  setProcessStepSelection,
  validateFlowchartGraph
} from "../../../dist/packages/oi-flowchart/src/index.js";

const graph = createFlowchartGraph(
  {
    nodes: [
      { id: "draft", label: "Draft" },
      { id: "review", label: "Review" },
      { id: "approved", label: "Approved" }
    ],
    transitions: [
      { id: "submit", from: "draft", to: "review", label: "Submit" },
      { id: "approve", from: "review", to: "approved", disabled: true }
    ]
  },
  { columns: 2, nodeWidth: 100, nodeHeight: 40 }
);

assert.equal(graph.validation.valid, true);
assert.equal(graph.nodes[1].x, 172);
assert.equal(graph.nodes[2].y, 96);
assert.equal(graph.transitions[0].points.length, 2);
assert.deepEqual(validateFlowchartGraph(graph), { valid: true, issues: [] });

const selected = selectProcessStep(graph, "review");
assert.equal(selected.incoming.length, 1);
assert.equal(selected.outgoing.length, 1);
assert.equal(selected.availableTransitions.length, 0);
assert.equal(selected.node.selected, true);

const updated = setProcessStepSelection(graph, "approved");
assert.equal(updated.nodes.find((node) => node.id === "approved").selected, true);
assert.equal(updated.nodes.find((node) => node.id === "draft").selected, false);

const invalid = createFlowchartGraph({
  nodes: [
    { id: 1, label: "A" },
    { id: 1, label: "Duplicate" }
  ],
  transitions: [
    { id: "bad", from: 1, to: 2 },
    { id: "loop", from: 1, to: 1 }
  ]
});
assert.equal(invalid.validation.valid, false);
assert.equal(invalid.validation.issues.some((issue) => issue.code === "duplicate_node"), true);
assert.equal(invalid.validation.issues.some((issue) => issue.code === "missing_target"), true);
assert.equal(invalid.validation.issues.some((issue) => issue.code === "self_transition"), true);
assert.throws(() => selectProcessStep(graph, "missing"), /process step not found/);

const sourceGraph = createFlowchartGraphFromDSL(
  [
    "st=>start: Start",
    "node10=>inputoutput: Draft|approved",
    "trans100=>condition: Submit",
    "node20=>subroutine: Review|approved",
    "e=>end: End",
    "st->node10->trans100",
    "trans100(yes)->node20->e"
  ].join("\n")
);
assert.equal(sourceGraph.validation.valid, true);
assert.equal(sourceGraph.nodes.find((node) => node.id === "node20").metadata.symbolType, "subroutine");
assert.equal(sourceGraph.transitions.some((transition) => transition.from === "trans100" && transition.to === "node20" && transition.label === "yes"), true);
