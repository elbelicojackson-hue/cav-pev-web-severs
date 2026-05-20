"use client";

import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Search, ExternalLink } from "lucide-react";
import { fetchPraxon } from "@/lib/api";
import type { Praxon } from "@/lib/types";
import Link from "next/link";

export default function ExplorerPage() {
  const [searchId, setSearchId] = useState("");
  const [results, setResults] = useState<Praxon[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSearch() {
    if (!searchId.trim()) return;
    setLoading(true);
    setError(null);
    try {
      const p = await fetchPraxon(searchId.trim());
      setResults([p]);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Not found");
      setResults([]);
    } finally {
      setLoading(false);
    }
  }

  function classVariant(pc: string) {
    return pc.startsWith("deliberation") ? "deliberation" : "operational";
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Praxon Explorer</h1>
        <p className="text-muted-foreground">Search and browse published praxons</p>
      </div>
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            placeholder="Enter praxon ID (e.g. prx_a1b2c3d4)"
            value={searchId}
            onChange={(e) => setSearchId(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleSearch()}
            className="w-full rounded-md border border-border bg-background py-2 pl-10 pr-4 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
          />
        </div>
        <Button onClick={handleSearch} disabled={loading}>
          {loading ? "Searching..." : "Search"}
        </Button>
      </div>
      {error && <p className="text-sm text-red-400">{error}</p>}
      <AnimatePresence>
        {results.map((p) => (
          <motion.div key={p.praxon_id} initial={{ opacity: 0, y: 10 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }}>
            <Card>
              <CardHeader className="flex flex-row items-center justify-between">
                <div>
                  <CardTitle className="font-mono text-base">{p.praxon_id}</CardTitle>
                  <p className="text-xs text-muted-foreground mt-1">Issued by {p.issuer} at {p.issued_at}</p>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant={classVariant(p.praxon_class)}>{p.praxon_class}</Badge>
                  <Link href={`/dashboard/praxon/detail?id=${p.praxon_id}`}>
                    <Button variant="ghost" size="icon"><ExternalLink className="h-4 w-4" /></Button>
                  </Link>
                </div>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <div>
                    <p className="text-xs font-medium text-muted-foreground mb-1">Causal Skeleton</p>
                    <p className="text-sm">{p.claim.causal_skeleton.subject} <span className="text-primary">{p.claim.causal_skeleton.relation}</span> {p.claim.causal_skeleton.object}</p>
                  </div>
                  <div>
                    <p className="text-xs font-medium text-muted-foreground mb-1">Confidence</p>
                    <p className="text-sm">{(p.claim.uncertainty_geometry.confidence * 100).toFixed(1)}%</p>
                  </div>
                  <div>
                    <p className="text-xs font-medium text-muted-foreground mb-1">Grounding</p>
                    <p className="text-sm">{p.grounding.length} handle(s)</p>
                  </div>
                </div>
              </CardContent>
            </Card>
          </motion.div>
        ))}
      </AnimatePresence>
    </div>
  );
}
