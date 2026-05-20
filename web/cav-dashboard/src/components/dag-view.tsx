"use client";

import { useMemo } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  type Node,
  type Edge,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { Praxon } from "@/lib/types";

interface DagViewProps {
  praxon: Praxon;
}

export function DagView({ praxon }: DagViewProps) {
  const { nodes, edges } = useMemo(() => {
    const nodes: Node[] = [];
    const edges: Edge[] = [];

    // Central node for this praxon
    nodes.push({
      id: praxon.praxon_id,
      position: { x: 300, y: 200 },
      data: { label: praxon.praxon_id },
      style: {
        background: "hsl(217 91% 60%)",
        color: "#fff",
        border: "none",
        borderRadius: "8px",
        padding: "8px 16px",
        fontSize: "12px",
        fontFamily: "monospace",
      },
    });

    // Derived-from nodes
    praxon.provenance.derived_from.forEach((parentId, i) => {
      nodes.push({
        id: parentId,
        position: { x: 100 + i * 200, y: 50 },
        data: { label: parentId },
        style: {
          background: "hsl(217 33% 17%)",
          color: "hsl(210 40% 96%)",
          border: "1px solid hsl(217 33% 25%)",
          borderRadius: "8px",
          padding: "8px 16px",
          fontSize: "11px",
          fontFamily: "monospace",
        },
      });
      edges.push({
        id: `e-${parentId}-${praxon.praxon_id}`,
        source: parentId,
        target: praxon.praxon_id,
        animated: true,
        style: { stroke: "hsl(217 91% 60%)" },
      });
    });

    // Grounding nodes
    praxon.grounding.forEach((g, i) => {
      const gId = `grounding-${i}`;
      nodes.push({
        id: gId,
        position: { x: 100 + i * 180, y: 380 },
        data: { label: `${g.type}` },
        style: {
          background: "hsl(142 71% 20%)",
          color: "hsl(142 71% 80%)",
          border: "1px solid hsl(142 71% 30%)",
          borderRadius: "8px",
          padding: "6px 12px",
          fontSize: "11px",
        },
      });
      edges.push({
        id: `e-${praxon.praxon_id}-${gId}`,
        source: praxon.praxon_id,
        target: gId,
        style: { stroke: "hsl(142 71% 45%)" },
      });
    });

    return { nodes, edges };
  }, [praxon]);

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      fitView
      proOptions={{ hideAttribution: true }}
      className="rounded-md"
    >
      <Background color="hsl(217 33% 20%)" gap={20} />
      <Controls />
    </ReactFlow>
  );
}
