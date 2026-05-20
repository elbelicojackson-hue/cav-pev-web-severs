# CAV Praxon Dashboard

Real-time monitoring dashboard for the CAV Praxon protocol network.

## Tech Stack

- **Next.js 15** (App Router)
- **React 19** + TypeScript
- **Tailwind CSS** (dark theme)
- **shadcn/ui** components (Radix primitives + CVA)
- **Framer Motion** animations
- **React Flow** (@xyflow/react) for DAG and network graph
- **SWR** for data fetching

## Pages

| Route | Description |
|-------|-------------|
| `/` | Dashboard home — stats cards + activity stream |
| `/explorer` | Praxon Explorer — search by ID, view summaries |
| `/praxon/[id]` | Praxon Detail — claim, grounding, provenance DAG |
| `/network` | Network Graph — peer topology visualization |
| `/audit` | Audit Log — filterable event history |

## Development

```bash
npm install
npm run dev
```

The dev server runs on `http://localhost:3000` and proxies `/api/*` requests to the Go backend at `http://localhost:8420`.

## Backend Connection

Configure the backend URL via `NEXT_PUBLIC_API_URL` env var, or rely on the built-in Next.js rewrite proxy (default: `localhost:8420`).

## Project Structure

```
src/
├── app/              # Next.js App Router pages
│   ├── page.tsx      # Dashboard home
│   ├── explorer/     # Praxon Explorer
│   ├── praxon/[id]/  # Praxon Detail
│   ├── network/      # Network Graph
│   └── audit/        # Audit Log
├── components/
│   ├── ui/           # shadcn/ui primitives (Card, Button, Badge)
│   ├── sidebar.tsx   # Navigation sidebar
│   └── dag-view.tsx  # React Flow DAG component
└── lib/
    ├── types.ts      # TypeScript types (mirrors Go types)
    ├── api.ts        # API client functions
    └── utils.ts      # cn() utility
```
