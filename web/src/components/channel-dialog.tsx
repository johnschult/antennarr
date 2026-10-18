import { Check, Loader2, Plus, Send, X } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { Choice } from '@/components/setup/choice'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useEdits } from '@/hooks/mutations'
import { CHANNEL_TYPES, NTFY_SERVER } from '@/lib/notifications'
import type { ChannelInput, ChannelType, NotifyChannel } from '@/lib/types'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** The channel being edited; undefined to add one. */
  channel?: NotifyChannel
}

export function ChannelDialog({ open, onOpenChange, channel }: Props) {
  // Remount the form whenever the target changes so its fields start fresh.
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg">
        {open && <ChannelForm key={channel?.id ?? '+'} channel={channel} onDone={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  )
}

function Field({ id, label, hint, children }: { id: string; label: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
    </div>
  )
}

function ChannelForm({ channel, onDone }: { channel?: NotifyChannel; onDone: () => void }) {
  const { channelAdd, channelReplace, channelTest } = useEdits()
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<ChannelType>(channel?.type ?? 'ntfy')
  // A webhook's address is never sent back to the browser, so it starts empty and empty means "keep it".
  const [url, setUrl] = useState(channel?.type === 'ntfy' ? (channel.url ?? NTFY_SERVER) : channel ? '' : NTFY_SERVER)
  const [topic, setTopic] = useState(channel?.topic ?? '')
  const [token, setToken] = useState('')
  const [user, setUser] = useState('')
  const [clearToken, setClearToken] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const busy = channelAdd.isPending || channelReplace.isPending

  // Saved secrets only carry over while the type is the one they were saved for.
  const saved = channel && channel.type === type ? channel : undefined
  const keep = 'Saved. Leave empty to keep it.'

  function pick(next: ChannelType) {
    setType(next)
    setToken('')
    setUser('')
    setClearToken(false)
    setUrl(next === 'ntfy' ? (channel?.type === 'ntfy' ? (channel.url ?? NTFY_SERVER) : NTFY_SERVER) : '')
  }

  /** What the form says now, as the server takes it. */
  function draft(): ChannelInput {
    const input: ChannelInput = { name, type }
    if (type !== 'pushover') input.url = url
    if (type === 'ntfy') input.topic = topic
    if (token) input.token = token
    if (type === 'pushover' && user) input.user = user
    if (clearToken && saved?.hasToken) input.clearToken = true
    return input
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      if (channel) await channelReplace.mutateAsync({ id: channel.id, input: draft() })
      else await channelAdd.mutateAsync(draft())
      onDone()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-4">
      <DialogHeader>
        <DialogTitle>{channel ? `Edit ${channel.name}` : 'Add a channel'}</DialogTitle>
        <DialogDescription>Where Antennarr sends messages. Tokens and keys are saved in the settings file and are never shown again.</DialogDescription>
      </DialogHeader>

      <Field id="channel-name" label="Name">
        <Input id="channel-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="My phone" maxLength={60} autoFocus />
      </Field>

      <Choice label="Type" value={type} onChange={pick} options={CHANNEL_TYPES} />

      {type === 'ntfy' && (
        <>
          <Field id="channel-url" label="Server" hint="https://ntfy.sh, or the address of your own ntfy server.">
            <Input id="channel-url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder={NTFY_SERVER} spellCheck={false} className="font-mono" />
          </Field>
          <Field id="channel-topic" label="Topic" hint="On ntfy.sh anyone who knows a topic can read it, so pick one that is hard to guess.">
            <Input id="channel-topic" value={topic} onChange={(e) => setTopic(e.target.value)} placeholder="antennarr-x7k2q9" spellCheck={false} className="font-mono" />
          </Field>
          <Field id="channel-token" label="Access token (optional)" hint="Only for a server that needs a login.">
            <Input id="channel-token" type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder={saved?.hasToken && !clearToken ? keep : ''} spellCheck={false} className="font-mono" />
          </Field>
        </>
      )}

      {type === 'pushover' && (
        <>
          <Field id="channel-token" label="Application token" hint="From the application you made at pushover.net/apps.">
            <Input id="channel-token" type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder={saved?.hasToken ? keep : ''} spellCheck={false} className="font-mono" />
          </Field>
          <Field id="channel-user" label="User key" hint="Shown on your Pushover dashboard.">
            <Input id="channel-user" type="password" value={user} onChange={(e) => setUser(e.target.value)} placeholder={saved?.hasUser ? keep : ''} spellCheck={false} className="font-mono" />
          </Field>
        </>
      )}

      {type === 'webhook' && (
        <>
          <Field id="channel-url" label="Webhook address" hint="Antennarr sends a JSON POST here with the event, title, message, device and time.">
            <Input id="channel-url" value={url} onChange={(e) => setUrl(e.target.value)} placeholder={saved?.host ? `Saved (${saved.host}). Leave empty to keep it.` : 'https://…'} spellCheck={false} className="font-mono" />
          </Field>
          <Field id="channel-token" label="Token (optional)" hint="Sent as a Bearer token in the Authorization header.">
            <Input id="channel-token" type="password" value={token} onChange={(e) => setToken(e.target.value)} placeholder={saved?.hasToken && !clearToken ? keep : ''} spellCheck={false} className="font-mono" />
          </Field>
        </>
      )}

      {type !== 'pushover' && saved?.hasToken && (
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input type="checkbox" checked={clearToken} onChange={(e) => setClearToken(e.target.checked)} className="size-3.5" />
          Remove the saved token
        </label>
      )}

      {error && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}

      <DialogFooter className="sm:justify-between">
        <Button type="button" variant="outline" disabled={channelTest.isPending} onClick={() => channelTest.mutate({ name: name.trim() || 'this channel', request: { id: channel?.id, channel: draft() } })}>
          {channelTest.isPending ? <Loader2 data-icon="inline-start" className="animate-spin" /> : <Send data-icon="inline-start" />}
          {channelTest.isPending ? 'Sending…' : 'Send test'}
        </Button>
        <div className="flex flex-col-reverse gap-2 sm:flex-row">
          <Button type="button" variant="outline" onClick={onDone}>
            <X data-icon="inline-start" /> Cancel
          </Button>
          <Button type="submit" disabled={busy}>
            {channel ? <Check data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
            {channel ? 'Save' : 'Add channel'}
          </Button>
        </div>
      </DialogFooter>
    </form>
  )
}
