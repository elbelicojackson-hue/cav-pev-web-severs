import type { Praxon, HealthStatus } from "./types";

const BASE = process.env.NEXT_PUBLIC_API_URL || "";

export async function fetchHealth(): Promise<HealthStatus> {
  const res = await fetch(`${BASE}/api/health`);
  if (!res.ok) throw new Error("Health check failed");
  return res.json();
}

export async function fetchGatewayHealth(): Promise<{ status: string; service: string; version: string }> {
  const res = await fetch(`${BASE}/v1/health`);
  if (!res.ok) throw new Error("Gateway health check failed");
  return res.json();
}

export async function fetchPraxon(id: string): Promise<Praxon> {
  const res = await fetch(`${BASE}/api/praxon/${id}`);
  if (!res.ok) throw new Error(`Praxon ${id} not found`);
  return res.json();
}

export async function publishPraxon(praxon: Praxon): Promise<{ ok: string; praxon_id: string; praxon_uri: string }> {
  const res = await fetch(`${BASE}/api/praxon`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(praxon),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text);
  }
  return res.json();
}

export async function fetchSubscribers(): Promise<{ subscribers: string[] }> {
  const res = await fetch(`${BASE}/api/subscribers`);
  if (!res.ok) throw new Error("Failed to fetch subscribers");
  return res.json();
}

export async function fetchCitizens(): Promise<{ citizens: Array<{
  did: string;
  fingerprint: string;
  level: number;
  capabilities?: { hypothesis_kinds?: string[]; tools?: string[]; description?: string; nickname?: string };
  verified_praxon_count: number;
  challenges_survived: number;
  registered_at: string;
  last_seen_at: string;
}> }> {
  const res = await fetch(`${BASE}/v1/citizens`);
  if (!res.ok) throw new Error("Failed to fetch citizens");
  return res.json();
}

export async function fetchNetworkStats(): Promise<{ citizens: number; level3: number; level2: number; level1: number }> {
  const res = await fetch(`${BASE}/v1/network/stats`);
  if (!res.ok) throw new Error("Failed to fetch network stats");
  return res.json();
}

export async function fetchPraxonList(): Promise<{ praxons: Array<{ praxon_id: string; issuer: string; praxon_class: string }>; total: number }> {
  const res = await fetch(`${BASE}/v1/praxon`);
  if (!res.ok) throw new Error("Failed to fetch praxon list");
  return res.json();
}
