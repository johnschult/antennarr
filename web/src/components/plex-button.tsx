import type { PlexFlow } from '@/hooks/use-plex-flow'
import { cn } from '@/lib/utils'

const asset = (name: string) => `${import.meta.env.BASE_URL}${name}`

/** The Plex wordmark. Its letters are light; pass `dark` to draw them black, for a light or Plex-yellow background. */
export function PlexWordmark({ className, dark }: { className?: string; dark?: boolean }) {
  return <img src={asset('plex-wordmark.png')} alt="Plex" className={cn('h-5 w-auto', dark && 'brightness-0', className)} draggable={false} />
}

/** The Plex app icon: a dark tile that reads on a light or dark page. */
export function PlexIcon({ className }: { className?: string }) {
  return <img src={asset('plex-icon.svg')} alt="" aria-hidden="true" className={cn('size-6 rounded-md', className)} draggable={false} />
}

/** "Sign in with Plex": Plex's own yellow-orange with black lettering, in either theme. */
export function PlexSignInButton({ onClick, busy, disabled }: { onClick: () => void; busy?: boolean; disabled?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy || disabled}
      className="inline-flex items-center gap-2.5 rounded-lg bg-[#e5a00d] px-5 py-3 text-sm font-semibold text-black shadow-sm transition-colors hover:bg-[#f0ae21] focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-60"
    >
      {busy ? 'Waiting for Plex…' : 'Sign in with'}
      {!busy && <PlexWordmark dark />}
    </button>
  )
}

/** What to tell the person while they sign in, or why it didn't work. */
export function PlexFlowMessage({ flow, onCancel, adding = false }: { flow: PlexFlow; onCancel: () => void; adding?: boolean }) {
  switch (flow.phase) {
    case 'waiting':
      return (
        <p role="status" className="text-sm text-muted-foreground">
          Finish signing in with Plex in the window that opened.{' '}
          <a href={flow.authUrl} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2 hover:text-foreground">
            Open it again
          </a>{' '}
          ·{' '}
          <button type="button" onClick={onCancel} className="underline underline-offset-2 hover:text-foreground">
            Cancel
          </button>
        </p>
      )
    case 'denied':
      return (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          The Plex account “{flow.name}” isn't allowed to open this page. Ask the owner to add it in Settings.
        </p>
      )
    case 'expired':
      return (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          That sign-in ran out. Try again.
        </p>
      )
    case 'error':
      return (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {flow.message}
        </p>
      )
    case 'done':
      return (
        <p role="status" className="text-sm text-emerald-700 dark:text-emerald-300">
          {adding ? `${flow.name} can now sign in.` : `Signed in as ${flow.name}.`}
        </p>
      )
    default:
      return null
  }
}
