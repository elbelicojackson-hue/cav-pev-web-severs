"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import {
  LayoutDashboard,
  List,
  Network,
  ScrollText,
  Activity,
  Users,
} from "lucide-react";

const navItems = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/dashboard/explorer", label: "Praxon Explorer", icon: List },
  { href: "/dashboard/network", label: "Citizens", icon: Users },
  { href: "/dashboard/exp", label: "EXP Capsules", icon: Network },
  { href: "/dashboard/audit", label: "Audit Log", icon: ScrollText },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="flex w-64 flex-col border-r border-border bg-card">
      <div className="flex items-center gap-2 border-b border-border px-6 py-4">
        <Activity className="h-6 w-6 text-primary" />
        <span className="text-lg font-bold tracking-tight">CAV Praxon</span>
      </div>
      <nav className="flex-1 space-y-1 px-3 py-4">
        {navItems.map((item) => {
          const Icon = item.icon;
          const active = pathname === item.href;
          return (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                active
                  ? "bg-primary/10 text-primary"
                  : "text-muted-foreground hover:bg-accent hover:text-foreground"
              )}
            >
              <Icon className="h-4 w-4" />
              {item.label}
            </Link>
          );
        })}
      </nav>
      <div className="border-t border-border px-6 py-3">
        <p className="text-xs text-muted-foreground">Protocol v1.0</p>
      </div>
    </aside>
  );
}
