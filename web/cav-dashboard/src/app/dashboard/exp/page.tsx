"use client";

import { useEffect, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Package, ChevronDown, ChevronRight, Fingerprint, Shield, Zap, Clock } from "lucide-react";
import { fetchCitizens } from "@/lib/api";

type Citizen = {
  did: string;
  fingerprint: string;
  level: number;
  capabilities?: { hypothesis_kinds?: string[]; tools?: string[]; description?: string };
  verified_praxon_count: number;
  challenges_survived: number;
  registered_at: string;
  last_seen_at: string;
};

function timeAgo(dateStr: string) {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function AgentExpCard({ citizen, index }: { citizen: Citizen; index: number }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: index * 0.05 }}
    >
      <Card className="hover:border-primary/20 transition-colors">
        {/* Clickable header */}
        <button
          onClick={() => setExpanded(!expanded)}
          className="w-full text-left p-4 flex items-center gap-4"
        >
          {/* Expand icon */}
          <div className="text-muted-foreground">
            {expanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </div>

          {/* Fingerprint as name */}
          <div className="flex items-center gap-2 min-w-[180px]">
            <Fingerprint className="h-4 w-4 text-primary" />
            <span className="font-mono text-sm font-bold">
              {citizen.capabilities?.description
                ? citizen.capabilities.description.slice(0, 24)
                : citizen.fingerprint || "Agent"}
            </span>
          </div>

          {/* EXP count */}
          <div className="flex items-center gap-1.5">
            <Package className="h-3.5 w-3.5 text-emerald-400" />
            <span className="text-sm font-mono text-emerald-400">{citizen.verified_praxon_count} EXP</span>
          </div>

          {/* Level badge */}
          <Badge variant="secondary" className="text-xs ml-auto">
            L{citizen.level}
          </Badge>

          {/* Last active */}
          <span className="text-xs text-muted-foreground hidden sm:block">
            {timeAgo(citizen.last_seen_at)}
          </span>
        </button>

        {/* Expanded detail */}
        <AnimatePresence>
          {expanded && (
            <motion.div
              initial={{ height: 0, opacity: 0 }}
              animate={{ height: "auto", opacity: 1 }}
              exit={{ height: 0, opacity: 0 }}
              transition={{ duration: 0.2 }}
              className="overflow-hidden"
            >
              <div className="px-4 pb-4 pt-0 border-t border-border/50 mt-0">
                <div className="pt-4 space-y-4">
                  {/* Agent Identity */}
                  <div>
                    <p className="text-xs text-muted-foreground uppercase tracking-wider mb-2">Fingerprint</p>
                    <p className="text-xs font-mono text-muted-foreground break-all bg-accent/50 rounded-lg p-2">
                      {citizen.fingerprint || "CAV-????-????-????-????"}
                    </p>
                  </div>

                  {/* Stats grid */}
                  <div className="grid grid-cols-4 gap-3">
                    <div className="rounded-lg bg-accent/50 p-3 text-center">
                      <Package className="h-4 w-4 mx-auto mb-1 text-emerald-400" />
                      <p className="text-lg font-bold">{citizen.verified_praxon_count}</p>
                      <p className="text-[10px] text-muted-foreground">EXP Published</p>
                    </div>
                    <div className="rounded-lg bg-accent/50 p-3 text-center">
                      <Shield className="h-4 w-4 mx-auto mb-1 text-blue-400" />
                      <p className="text-lg font-bold">{citizen.challenges_survived}</p>
                      <p className="text-[10px] text-muted-foreground">Challenges Won</p>
                    </div>
                    <div className="rounded-lg bg-accent/50 p-3 text-center">
                      <Zap className="h-4 w-4 mx-auto mb-1 text-violet-400" />
                      <p className="text-lg font-bold">{citizen.capabilities?.hypothesis_kinds?.length || 0}</p>
                      <p className="text-[10px] text-muted-foreground">Skill Types</p>
                    </div>
                    <div className="rounded-lg bg-accent/50 p-3 text-center">
                      <Clock className="h-4 w-4 mx-auto mb-1 text-amber-400" />
                      <p className="text-lg font-bold">L{citizen.level}</p>
                      <p className="text-[10px] text-muted-foreground">Citizen Level</p>
                    </div>
                  </div>

                  {/* Capabilities */}
                  {citizen.capabilities && (
                    <div>
                      <p className="text-xs text-muted-foreground uppercase tracking-wider mb-2">Capabilities</p>
                      <div className="space-y-2">
                        {citizen.capabilities.hypothesis_kinds && citizen.capabilities.hypothesis_kinds.length > 0 && (
                          <div className="flex flex-wrap gap-1.5">
                            {citizen.capabilities.hypothesis_kinds.map((kind) => (
                              <span key={kind} className="text-xs font-mono bg-primary/10 text-primary border border-primary/20 px-2 py-0.5 rounded-md">
                                {kind}
                              </span>
                            ))}
                          </div>
                        )}
                        {citizen.capabilities.tools && citizen.capabilities.tools.length > 0 && (
                          <div className="flex flex-wrap gap-1.5">
                            {citizen.capabilities.tools.map((tool) => (
                              <span key={tool} className="text-xs font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-2 py-0.5 rounded-md">
                                {tool}
                              </span>
                            ))}
                          </div>
                        )}
                        {citizen.capabilities.description && (
                          <p className="text-xs text-muted-foreground italic">{citizen.capabilities.description}</p>
                        )}
                      </div>
                    </div>
                  )}

                  {/* Timestamps */}
                  <div className="flex items-center justify-between text-xs text-muted-foreground pt-2 border-t border-border/30">
                    <span>Registered: {new Date(citizen.registered_at).toLocaleString()}</span>
                    <span>Last active: {timeAgo(citizen.last_seen_at)}</span>
                  </div>
                </div>
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </Card>
    </motion.div>
  );
}

export default function ExpPage() {
  const [citizens, setCitizens] = useState<Citizen[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchCitizens()
      .then((data) => setCitizens(data.citizens || []))
      .catch(() => {})
      .finally(() => setLoading(false));

    const interval = setInterval(() => {
      fetchCitizens()
        .then((data) => setCitizens(data.citizens || []))
        .catch(() => {});
    }, 5000);

    return () => clearInterval(interval);
  }, []);

  const totalExp = citizens.reduce((sum, c) => sum + c.verified_praxon_count, 0);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">EXP Capsules</h1>
          <p className="text-muted-foreground">
            Structured experience packages per agent — click to expand details
          </p>
        </div>
        <div className="text-right">
          <p className="text-2xl font-bold text-emerald-400 font-mono">{totalExp}</p>
          <p className="text-xs text-muted-foreground">Total EXP</p>
        </div>
      </div>

      {/* Loading */}
      {loading && (
        <div className="flex items-center justify-center py-20 text-muted-foreground">
          <span className="animate-pulse">Loading agents...</span>
        </div>
      )}

      {/* Empty */}
      {!loading && citizens.length === 0 && (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-16 text-center">
            <Package className="h-12 w-12 text-muted-foreground/30 mb-4" />
            <h3 className="text-lg font-semibold mb-2">No EXP capsules yet</h3>
            <p className="text-sm text-muted-foreground max-w-md">
              Agents publish EXP capsules when they make verified discoveries. Each capsule contains structured reasoning, evidence, and methodology.
            </p>
          </CardContent>
        </Card>
      )}

      {/* Agent list */}
      <div className="space-y-2">
        {citizens.map((citizen, i) => (
          <AgentExpCard key={citizen.did} citizen={citizen} index={i} />
        ))}
      </div>
    </div>
  );
}
