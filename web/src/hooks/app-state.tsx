import type { ReactNode } from 'react'
import { Logo } from '@/components/logo'
import { Button } from '@/components/ui/button'
import { ApiError } from '@/lib/api'
import { AppContext } from './app-context'
import { useLiveState } from './use-live-state'

/** Loads the app state once, keeps it live, and holds the page back until it arrives. */
export function AppStateProvider({ children }: { children: ReactNode }) {
  const { state, connected, error, isLoading } = useLiveState()

  if (!state) {
    return <Splash error={isLoading ? undefined : error} />
  }
  return <AppContext.Provider value={{ state, connected }}>{children}</AppContext.Provider>
}

function Splash({ error }: { error?: Error | null }) {
  const forbidden = error instanceof ApiError && error.status === 403
  return (
    <div className="grid min-h-screen place-items-center px-4">
      <div className="max-w-md text-center">
        <div className="mx-auto mb-4 flex w-fit items-end gap-3">
          <Logo size={56} />
          <span className="font-wordmark pb-2 text-4xl leading-none font-bold tracking-[0.02em]">Antennarr</span>
        </div>
        {!error ? (
          <p className="text-muted-foreground">Connecting to Antennarr…</p>
        ) : forbidden ? (
          <>
            <h1 className="text-lg font-semibold">This address can't open the admin page</h1>
            <p className="mt-2 text-sm text-muted-foreground">
              Open it from the machine running Antennarr, or add this computer's address to <code>admin</code> in the
              settings file and reload.
            </p>
          </>
        ) : (
          <>
            <h1 className="text-lg font-semibold">Can't reach Antennarr</h1>
            <p className="mt-2 text-sm text-muted-foreground">{error.message}</p>
            <Button className="mt-4" variant="outline" onClick={() => window.location.reload()}>
              Try again
            </Button>
          </>
        )}
      </div>
    </div>
  )
}
