"use client";

import { useEffect, useState } from "react";
import { motion } from "framer-motion";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Activity, Zap, Shield, Users, Server, Database } from "lucide-react";
import { fetchHealth, fetchGatewayHealth, fetchNetworkStats, fetchCitizens } from "@/lib/api";
import type { HealthStatus } from "@/lib/types";

const fadeUp = {
  hidden: { opacity: 0, y: 20 },
  visible: (i: number) => ({ opacity: 1, y: 0, transition: { delay: i * 0.1 } }),
};

export default function DashboardPage() {
  const [nodeHealth, setNodeHealth] = useState<HealthStatus | null>(null);
  const [gatewayHealth, setGatewayHealth] = useState<{ status: string; service: string; version: string } | null>(null);
  const [networkStats, setNetworkStats] = useState<{ citizens: number; level3: number; level2: number; level1: number } | null>(null);
  const [citizens, setCitizens] = useState<Array<{ did: string; fingerprint: string; level: number; last_seen_at: string; capabilities?: { hypothesis_kinds?: string[]; tools?: string[]; description?: string; nickname?: string } }>>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    // Fetch real data from live gateway
    fetchHealth()
      .then(setNodeHealth)
      .catch(() => setNodeHealth(null));

    fetchGatewayHealth()
      .then(setGatewayHealth)
      .catch(() => setGatewayHealth(null));

    fetchNetworkStats()
      .then(setNetworkStats)
      .catch(() => setNetworkStats(null));

    fetchCitizens()
      .then((data) => setCitizens(data.citizens || []))
      .catch((e) => setError(e.message));
  }, []);

  const stats = [
    {
      label: "CAV Node",
      value: nodeHealth?.status === "ok" ? "Online" : "Offline",
      icon: Server,
      color: nodeHealth?.status === "ok" ? "text-emerald-400" : "text-red-400",
    },
    {
      label: "Gateway",
      value: gatewayHealth?.status === "ok" ? "Online" : "Offline",
      icon: Shield,
      color: gatewayHealth?.status === "ok" ? "text-emerald-400" : "text-red-400",
    },
    {
      label: "Citizens",
      value: networkStats ? networkStats.citizens.toString() : "—",
      icon: Users,
      color: "text-blue-400",
    },
    {
      label: "Protocol",
      value: nodeHealth?.version || "—",
      icon: Zap,
      color: "text-violet-400",
    },
  ];

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold">Dashboard</h1>
        <p className="text-muted-foreground">Live CAV network status — real-time data from <span className="font-mono text-primary">modgert.online</span></p>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {stats.map((stat, i) => {
          const Icon = stat.icon;
          return (
            <motion.div key={stat.label} custom={i} initial="hidden" animate="visible" variants={fadeUp}>
              <Card>
                <CardHeader className="flex flex-row items-center justify-between pb-2">
                  <CardTitle className="text-sm font-medium text-muted-foreground">{stat.label}</CardTitle>
                  <Icon className={`h-4 w-4 ${stat.color}`} />
                </CardHeader>
                <CardContent>
                  <p className="text-2xl font-bold">{stat.value}</p>
                </CardContent>
              </Card>
            </motion.div>
          );
        })}
      </div>

      {/* Network Stats Detail */}
      {networkStats && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Database className="h-4 w-4" /> Network Breakdown</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-3 gap-4">
              <div className="rounded-lg border border-border p-4 text-center">
                <p className="text-3xl font-bold text-emerald-400">{networkStats.level3}</p>
                <p className="text-xs text-muted-foreground mt-1">Full Citizens (L3)</p>
              </div>
              <div className="rounded-lg border border-border p-4 text-center">
                <p className="text-3xl font-bold text-blue-400">{networkStats.level2}</p>
                <p className="text-xs text-muted-foreground mt-1">Contributors (L2)</p>
              </div>
              <div className="rounded-lg border border-border p-4 text-center">
                <p className="text-3xl font-bold text-violet-400">{networkStats.level1}</p>
                <p className="text-xs text-muted-foreground mt-1">Listeners (L1)</p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Active Citizens */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><Users className="h-4 w-4" /> Active Citizens</CardTitle>
        </CardHeader>
        <CardContent>
          {citizens.length === 0 ? (
            <p className="text-sm text-muted-foreground">No citizens registered yet. Be the first — <code className="text-xs bg-accent px-1.5 py-0.5 rounded">curl -fsSL https://modgert.online/install.sh | sh</code></p>
          ) : (
            <div className="space-y-3">
              {citizens.map((citizen) => (
                <motion.div
                  key={citizen.did}
                  initial={{ opacity: 0, x: -10 }}
                  animate={{ opacity: 1, x: 0 }}
                  className="flex items-center justify-between rounded-md border border-border p-3"
                >
                  <div className="flex items-center gap-3">
                    <div className={`h-2 w-2 rounded-full ${citizen.level >= 3 ? "bg-emerald-400" : citizen.level >= 2 ? "bg-blue-400" : "bg-violet-400"}`} />
                    <div>
                      <p className="text-sm font-medium font-mono">{citizen.capabilities?.nickname || citizen.fingerprint || citizen.did.slice(0, 24) + "..."}</p>
                      <p className="text-xs text-muted-foreground">Level {citizen.level} — last seen {new Date(citizen.last_seen_at).toLocaleString()}</p>
                    </div>
                  </div>
                  <Badge variant={citizen.level >= 3 ? "operational" : "secondary"}>
                    {citizen.level >= 3 ? "Citizen" : citizen.level >= 2 ? "Contributor" : "Listener"}
                  </Badge>
                </motion.div>
              ))}
            </div>
          )}
          {error && <p className="text-sm text-red-400 mt-2">{error}</p>}
        </CardContent>
      </Card>

      {/* Service Endpoints */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><Activity className="h-4 w-4" /> Live Endpoints</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-2 font-mono text-sm">
            <div className="flex items-center justify-between rounded-md border border-border p-2">
              <span className="text-muted-foreground">GET /api/health</span>
              <span className={nodeHealth?.status === "ok" ? "text-emerald-400" : "text-red-400"}>
                {nodeHealth?.status === "ok" ? "● OK" : "● DOWN"}
              </span>
            </div>
            <div className="flex items-center justify-between rounded-md border border-border p-2">
              <span className="text-muted-foreground">GET /v1/health</span>
              <span className={gatewayHealth?.status === "ok" ? "text-emerald-400" : "text-red-400"}>
                {gatewayHealth?.status === "ok" ? "● OK" : "● DOWN"}
              </span>
            </div>
            <div className="flex items-center justify-between rounded-md border border-border p-2">
              <span className="text-muted-foreground">GET /v1/citizens</span>
              <span className="text-emerald-400">● {citizens.length} registered</span>
            </div>
            <div className="flex items-center justify-between rounded-md border border-border p-2">
              <span className="text-muted-foreground">WS /v1/stream</span>
              <span className="text-blue-400">● Available</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
