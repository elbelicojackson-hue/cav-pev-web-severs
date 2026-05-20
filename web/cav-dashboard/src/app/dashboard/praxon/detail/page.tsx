"use client";

import { useEffect, useState, Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { motion } from "framer-motion";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ArrowLeft, GitBranch, Shield, FlaskConical } from "lucide-react";
import { fetchPraxon } from "@/lib/api";
import type { Praxon } from "@/lib/types";
import Link from "next/link";
import { DagView } from "@/components/dag-view";

function PraxonDetail() {
  const searchParams = useSearchParams();
  const id = searchParams.get("id");
  const [praxon, setPraxon] = useState<Praxon | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (id) {
      fetchPraxon(id).then(setPraxon).catch((e) => setError(e.message));
    }
  }, [id]);

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4">
        <p className="text-red-400">{error}</p>
        <Link href="/dashboard/explorer"><Button variant="ghost"><ArrowLeft className="mr-2 h-4 w-4" />Back to Explorer</Button></Link>
      </div>
    );
  }

  if (!praxon) {
    return <div className="flex items-center justify-center h-full text-muted-foreground">Loading...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Link href="/dashboard/explorer"><Button variant="ghost" size="icon"><ArrowLeft className="h-4 w-4" /></Button></Link>
        <div>
          <h1 className="text-2xl font-bold font-mono">{praxon.praxon_id}</h1>
          <p className="text-muted-foreground text-sm">Issued by {praxon.issuer}</p>
        </div>
        <Badge variant={praxon.praxon_class.startsWith("deliberation") ? "deliberation" : "operational"} className="ml-auto">
          {praxon.praxon_class}
        </Badge>
      </div>
      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }}>
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><FlaskConical className="h-4 w-4" /> Claim</CardTitle></CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Causal Skeleton</h4>
                <div className="rounded-md border border-border p-3 text-sm space-y-1">
                  <p><span className="text-muted-foreground">Subject:</span> {praxon.claim.causal_skeleton.subject}</p>
                  <p><span className="text-muted-foreground">Relation:</span> <span className="text-primary">{praxon.claim.causal_skeleton.relation}</span></p>
                  <p><span className="text-muted-foreground">Object:</span> {praxon.claim.causal_skeleton.object}</p>
                  <p><span className="text-muted-foreground">Mechanism:</span> {praxon.claim.causal_skeleton.mechanism_hypothesis}</p>
                  <p><span className="text-muted-foreground">Strength:</span> {praxon.claim.causal_skeleton.strength}</p>
                </div>
              </div>
              <div className="space-y-2">
                <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Uncertainty Geometry</h4>
                <div className="rounded-md border border-border p-3 text-sm space-y-1">
                  <p><span className="text-muted-foreground">Confidence:</span> {(praxon.claim.uncertainty_geometry.confidence * 100).toFixed(1)}%</p>
                  <p><span className="text-muted-foreground">Counterfactual:</span> {praxon.claim.uncertainty_geometry.counterfactual_neighborhood}</p>
                  <p><span className="text-muted-foreground">Failure Modes:</span> {praxon.claim.uncertainty_geometry.known_failure_modes.join(", ") || "None"}</p>
                </div>
              </div>
            </div>
            <div className="space-y-2">
              <h4 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Falsifiability</h4>
              <div className="rounded-md border border-border p-3 text-sm">
                <p><span className="text-muted-foreground">Would be retracted if:</span> {praxon.claim.falsifiability.would_be_retracted_if}</p>
                {praxon.claim.falsifiability.test_protocol_praxon_ref && (
                  <p><span className="text-muted-foreground">Test protocol:</span> {praxon.claim.falsifiability.test_protocol_praxon_ref}</p>
                )}
              </div>
            </div>
          </CardContent>
        </Card>
      </motion.div>
      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.1 }}>
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><Shield className="h-4 w-4" /> Grounding ({praxon.grounding.length})</CardTitle></CardHeader>
          <CardContent>
            <div className="space-y-2">
              {praxon.grounding.map((g, i) => (
                <div key={i} className="rounded-md border border-border p-3">
                  <Badge variant="secondary" className="mb-2">{g.type}</Badge>
                  <pre className="text-xs text-muted-foreground overflow-x-auto">{JSON.stringify(g, null, 2)}</pre>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </motion.div>
      <motion.div initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} transition={{ delay: 0.2 }}>
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><GitBranch className="h-4 w-4" /> Provenance DAG</CardTitle></CardHeader>
          <CardContent className="h-[400px]">
            <DagView praxon={praxon} />
          </CardContent>
        </Card>
      </motion.div>
    </div>
  );
}

export default function PraxonDetailPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center h-full text-muted-foreground">Loading...</div>}>
      <PraxonDetail />
    </Suspense>
  );
}
