import { useQueryClient } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { Logo } from '@/components/logo'
import { PlexFlowMessage, PlexSignInButton } from '@/components/plex-button'
import { Button } from '@/components/ui/button'
import { ApiError } from '@/lib/api'
import { AppContext } from './app-context'
import { AUTH_KEY, useAuthStatus } from './use-auth'
import { usePlexFlow } from './use-plex-flow'
import { STATE_KEY, useLiveState } from './use-live-state'

/** Loads the app state once, keeps it live, and holds the page back until it arrives. */
export function AppStateProvider({ children }: { children: ReactNode }) {
  const { state, connected, error, isLoading } = useLiveState()

  // Refused while the page is open (a Plex sign-in ended, say): go back to the
  // sign-in screen rather than sit on stale numbers.
  const refused = error instanceof ApiError && error.status === 403
  if (!state || refused) {
    return <Splash error={isLoading && !refused ? undefined : error} />
  }
  return <AppContext.Provider value={{ state, connected }}>{children}</AppContext.Provider>
}

function Splash({ error }: { error?: Error | null }) {
  const forbidden = error instanceof ApiError && error.status === 403
  const qc = useQueryClient()
  const auth = useAuthStatus(forbidden)
  const { flow, start, cancel } = usePlexFlow({
    onDone: () => {
      void qc.invalidateQueries({ queryKey: AUTH_KEY })
      void qc.invalidateQueries({ queryKey: STATE_KEY }) // now allowed in: fetch the page's data
    },
  })
  const busy = flow.phase === 'starting' || flow.phase === 'waiting'
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
            {auth.data?.plexEnabled ? (
              <>
                <p className="mt-2 text-sm text-muted-foreground">Sign in with a Plex account that has been allowed to administer Antennarr.</p>
                <div className="mt-5 flex justify-center">
                  <PlexSignInButton onClick={start} busy={busy} />
                </div>
                <div className="mt-3 min-h-5">
                  <PlexFlowMessage flow={flow} onCancel={cancel} />
                </div>
                <p className="mt-4 text-xs text-muted-foreground">
                  Or open it from the machine running Antennarr, or add this computer's address to <code>admin</code> in the settings file.
                </p>
              </>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">
                Open it from the machine running Antennarr, or add this computer's address to <code>admin</code> in the
                settings file and reload.
              </p>
            )}
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
