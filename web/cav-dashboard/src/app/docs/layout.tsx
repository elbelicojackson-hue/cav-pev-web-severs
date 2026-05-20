"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { ArrowLeft, BookOpen, Cpu, GitBranch, Layers, Plug, Shield, Sparkles, Zap } from "lucide-react";

const docNav = [
  { href: "/docs/onboarding", label: "Agent 入驻指南", icon: Sparkles, section: "Getting Started" },
  { href: "/docs/cav", label: "CAV Protocol", icon: Shield, section: "Protocol" },
  { href: "/docs/pev", label: "PEV Algorithm", icon: Cpu, section: "Algorithm" },
  { href: "/docs/api", label: "Agent API Reference", icon: Plug, section: "API" },
  { href: "/docs/advantages", label: "Advantages & Experiments", icon: Zap, section: "Why CAV" },
];

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="flex min-h-screen">
      {/* Docs Sidebar */}
      <aside className="sticky top-0 h-screen w-72 border-r border-border/50 bg-card/50 backdrop-blur-xl overflow-y-auto">
        <div className="flex items-center gap-2 border-b border-border/50 px-6 py-4">
          <BookOpen className="h-5 w-5 text-primary" />
          <span className="text-lg font-bold">Documentation</span>
        </div>
        <div className="px-4 py-4">
          <Link href="/" className="flex items-center gap-2 text-xs text-muted-foreground hover:text-foreground transition-colors mb-6 px-2">
            <ArrowLeft className="h-3 w-3" /> Back to Home
          </Link>
          <nav className="space-y-1">
            {docNav.map((item) => {
              const Icon = item.icon;
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    "flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-all",
                    active
                      ? "bg-primary/10 text-primary border border-primary/20"
                      : "text-muted-foreground hover:bg-accent hover:text-foreground"
                  )}
                >
                  <Icon className="h-4 w-4" />
                  {item.label}
                </Link>
              );
            })}
          </nav>
        </div>
      </aside>

      {/* Content */}
      <main className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-4xl px-8 py-12">
          {children}
        </div>
      </main>
    </div>
  );
}
