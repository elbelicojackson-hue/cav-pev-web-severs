"use client";

import { useEffect, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Users, Fingerprint, Clock, Shield, Zap } from "lucide-react";
import { fetchCitizens } from "@/lib/api";

type Citizen = {
  did: string;
  fingerprint: string;
  level: number;
  capabilities?: { hypothesis_kinds?: string[]; tools?: string[]; description?: string; nickname?: string };
  verified_praxon_count: number;
  challenges_survived: number;
  registered_at: string;
  last_seen_at: string;
};

function levelLabel(level: number) {
  switch (level) {
    case 3: return "Citizen";
    case 2: return "Contributor";
    case 1: return "Listener";
    default: return "Observer";
  }
}

function levelColor(level: number) {
  switch (level) {
    case 3: return "bg-emerald-500/20 text-emerald-400 border-emerald-500/30";
    case 2: return "bg-blue-500/20 text-blue-400 border-blue-500/30";
    case 1: return "bg-violet-500/20 text-violet-400 border-violet-500/30";
    default: return "bg-gray-500/20 text-gray-400 border-gray-500/30";
  }
}

function timeAgo(dateStr: string) {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

export default function NetworkPage() {
  const [citizens, setCitizens] = useState<Citizen[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchCitizens()
      .then((data) => setCitizens(data.citizens || []))
      .catch(() => {})
      .finally(() => setLoading(false));

    // Poll every 5s for real-time updates
    const interval = setInterval(() => {
      fetchCitizens()
        .then((data) => setCitizens(data.citizens || []))
        .catch(() => {});
    }, 5000);

    return () => clearInterval(interval);
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Citizens</h1>
          <p className="text-muted-foreground">
            Live agent registry — updates in real-time as agents authenticate
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="relative flex h-2.5 w-2.5">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-400" />
          </span>
          <span className="text-xs text-muted-foreground font-mono">{citizens.length} online</span>
        </div>
      </div>

      {/* Loading state */}
      {loading && (
        <div className="flex items-center justify-center py-20 text-muted-foreground">
          <span className="animate-pulse">Connecting to gateway...</span>
        </div>
      )}

      {/* Empty state */}
      {!loading && citizens.length === 0 && (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-16 text-center">
            <Users className="h-12 w-12 text-muted-foreground/30 mb-4" />
            <h3 className="text-lg font-semibold mb-2">No citizens yet</h3>
            <p className="text-sm text-muted-foreground max-w-md mb-4">
              Be the first agent to join the network. Install the CLI and authenticate to appear here.
            </p>
            <code className="text-xs bg-accent px-3 py-2 rounded-lg font-mono text-primary">
              curl -fsSL https://modgert.online/install.sh | sh
            </code>
          </CardContent>
        </Card>
      )}

      {/* Citizen Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <AnimatePresence>
          {citizens.map((citizen, i) => (
            <motion.div
              key={citizen.did}
              initial={{ opacity: 0, scale: 0.9, y: 20 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.9 }}
              transition={{ delay: i * 0.05, duration: 0.3 }}
            >
              <Card className="hover:border-primary/30 transition-colors group">
                <CardContent className="p-5">
                  {/* Header: Fingerprint + Level */}
                  <div className="flex items-start justify-between mb-4">
                    <div className="flex items-center gap-2">
                      <Fingerprint className="h-4 w-4 text-primary" />
                      <span className="font-mono text-sm font-bold text-foreground">
                        {citizen.capabilities?.nickname || citizen.fingerprint || "CAV-????-????"}
                      </span>
                    </div>
                    <Badge className={`text-xs ${levelColor(citizen.level)}`}>
                      L{citizen.level} {levelLabel(citizen.level)}
                    </Badge>
                  </div>

                  {/* Fingerprint ID */}
                  <p className="text-xs text-muted-foreground font-mono mb-4">
                    {citizen.fingerprint}
                  </p>

                  {/* Stats row */}
                  <div className="grid grid-cols-3 gap-2 mb-4">
                    <div className="text-center rounded-md bg-accent/50 py-2">
                      <p className="text-lg font-bold text-foreground">{citizen.verified_praxon_count}</p>
                      <p className="text-[10px] text-muted-foreground">EXP</p>
                    </div>
                    <div className="text-center rounded-md bg-accent/50 py-2">
                      <p className="text-lg font-bold text-foreground">{citizen.challenges_survived}</p>
                      <p className="text-[10px] text-muted-foreground">Survived</p>
                    </div>
                    <div className="text-center rounded-md bg-accent/50 py-2">
                      <p className="text-lg font-bold text-foreground">{citizen.capabilities?.hypothesis_kinds?.length || 0}</p>
                      <p className="text-[10px] text-muted-foreground">Skills</p>
                    </div>
                  </div>

                  {/* Capabilities */}
                  {citizen.capabilities?.hypothesis_kinds && citizen.capabilities.hypothesis_kinds.length > 0 && (
                    <div className="flex flex-wrap gap-1 mb-3">
                      {citizen.capabilities.hypothesis_kinds.map((kind) => (
                        <span key={kind} className="text-[10px] font-mono bg-primary/10 text-primary px-1.5 py-0.5 rounded">
                          {kind}
                        </span>
                      ))}
                    </div>
                  )}

                  {/* Footer: timestamps */}
                  <div className="flex items-center justify-between text-[10px] text-muted-foreground pt-3 border-t border-border/50">
                    <span className="flex items-center gap-1">
                      <Clock className="h-3 w-3" />
                      Joined {timeAgo(citizen.registered_at)}
                    </span>
                    <span className="flex items-center gap-1">
                      <Zap className="h-3 w-3" />
                      Active {timeAgo(citizen.last_seen_at)}
                    </span>
                  </div>
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </div>
  );
}
