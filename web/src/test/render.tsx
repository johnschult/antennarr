import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { MemoryRouter } from 'react-router'
import { Toaster } from '@/components/ui/sonner'
import { AppContext } from '@/hooks/app-context'
import type { AppState } from '@/lib/types'

/** Render a page with a fixed app state, no network, and a router. */
export function renderWithApp(ui: ReactElement, state: AppState, opts: { connected?: boolean; route?: string } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return {
    client,
    ...render(
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[opts.route ?? '/']}>
          <AppContext.Provider value={{ state, connected: opts.connected ?? true }}>{ui}</AppContext.Provider>
        </MemoryRouter>
        <Toaster />
      </QueryClientProvider>,
    ),
  }
}

/** Replace fetch with a stub that answers from `routes` (`"METHOD /path"` -> handler). */
export function stubFetch(routes: Record<string, (body: unknown) => { status?: number; body?: unknown }>) {
  const calls: { method: string; url: string; body: unknown }[] = []
  const fn = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    const body = init?.body ? JSON.parse(String(init.body)) : undefined
    calls.push({ method, url, body })
    const key = `${method} ${url.replace(/^.*?\/api(?=\/)/, '')}` // whatever the base path is
    const handler = routes[key]
    if (!handler) return new Response(JSON.stringify({ error: `unexpected ${key}` }), { status: 500 })
    const r = handler(body)
    const status = r.status ?? 200
    return new Response(status === 204 ? null : JSON.stringify(r.body ?? {}), { status })
  }
  return { fn, calls }
}
