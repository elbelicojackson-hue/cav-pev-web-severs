"use client";

import { useState } from "react";
import { motion } from "framer-motion";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { ScrollText, Filter } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { AuditEntry } from "@/lib/types";

const MOCK_AUDIT: AuditEntry[] = [
  { timestamp: new Date().toISOString(), event: "published", praxon_id: "prx_a1b2c3", issuer: "agent:pev-alpha", detail: "" },
  { timestamp: new Date(Date.now() - 5000).toISOString(), event: "fetched", praxon_id: "prx_a1b2c3", issuer: "", detail: "" },
  { timestamp: new Date(Date.now() - 15000).toISOString(), event: "announcement_received", praxon_id: "prx_d4e5f6", issuer: "agent:causal-beta", detail: "" },
  { timestamp: new Date(Date.now() - 30000).toISOString(), event: "publish_rejected", praxon_id: "", issuer: "agent:untrusted", detail: "missing required field: claim.causal_skeleton.subject" },
  { timestamp: new Date(Date.now() - 60000).toISOString(), event: "publish_rate_limited", praxon_id: "prx_x9y8z7", issuer: "agent:spammer", detail: "" },
  { timestamp: new Date(Date.now() - 120000).toISOString(), event: "published", praxon_id: "prx_d4e5f6", issuer: "agent:causal-beta", detail: "" },
];

function eventBadgeVariant(event: string) {
  if (event === "published") return "operational" as const;
  if (event.includes("rejected") || event.includes("rate_limited")) return "outline" as const;
  return "secondary" as const;
}

export default function AuditPage() {
  const [filter, setFilter] = useState<string | null>(null);
  const filtered = filter ? MOCK_AUDIT.filter((e) => e.event === filter) : MOCK_AUDIT;
  const eventTypes = [...new Set(MOCK_AUDIT.map((e) => e.event))];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Audit Log</h1>
        <p className="text-muted-foreground">Protocol event history and compliance trail</p>
      </div>
      <div className="flex items-center gap-2 flex-wrap">
        <Filter className="h-4 w-4 text-muted-foreground" />
        <Button variant={filter === null ? "default" : "ghost"} size="sm" onClick={() => setFilter(null)}>All</Button>
        {eventTypes.map((et) => (
          <Button key={et} variant={filter === et ? "default" : "ghost"} size="sm" onClick={() => setFilter(et)}>{et}</Button>
        ))}
      </div>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><ScrollText className="h-4 w-4" /> Events ({filtered.length})</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-1">
            <div className="grid grid-cols-[180px_150px_1fr_120px] gap-2 px-3 py-2 text-xs font-medium text-muted-foreground uppercase tracking-wider border-b border-border">
              <span>Timestamp</span><span>Event</span><span>Details</span><span>Praxon ID</span>
            </div>
            {filtered.map((entry, i) => (
              <motion.div
                key={`${entry.timestamp}-${i}`}
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: i * 0.03 }}
                className="grid grid-cols-[180px_150px_1fr_120px] gap-2 px-3 py-2 text-sm rounded-md hover:bg-accent/50 transition-colors"
              >
                <span className="text-xs text-muted-foreground font-mono">{new Date(entry.timestamp).toLocaleTimeString()}</span>
                <span><Badge variant={eventBadgeVariant(entry.event)}>{entry.event}</Badge></span>
                <span className="text-muted-foreground truncate">
                  {entry.issuer && <span>issuer: {entry.issuer}</span>}
                  {entry.detail && <span className="text-red-400 ml-2">{entry.detail}</span>}
                </span>
                <span className="font-mono text-xs">{entry.praxon_id || "—"}</span>
              </motion.div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
