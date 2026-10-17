import { useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useApp } from '@/hooks/app-context'
import { useEdits } from '@/hooks/mutations'
import { parseAddresses } from '@/lib/format'
import type { SettingsPatch } from '@/lib/types'

export function SettingsPage() {
  const { state } = useApp()
  const { settings, tuner, editable } = state

  return (
    <div className="max-w-3xl space-y-4">
      <h1 className="text-xl font-semibold tracking-tight">Settings</h1>

      <NumberCard
        key={`reserve-${settings.ownerReserve}`}
        title="Owner reserve"
        description="Tuners always kept free for your own TVs and apps. Viewers are refused instead of taking the last ones."
        label="Tuners to keep free"
        initial={settings.ownerReserve}
        max={tuner.tunerCount}
        disabled={!editable}
        field="ownerReserve"
        hint={`Your tuner has ${tuner.tunerCount}.`}
      />
      <NumberCard
        key={`cap-${settings.sharedCap}`}
        title="Shared cap"
        description="The most tuners all viewers together may use, however many are idle."
        label="Tuners for viewers"
        initial={settings.sharedCap}
        max={tuner.tunerCount}
        disabled={!editable}
        field="sharedCap"
        hint="0 means no cap beyond the owner reserve."
      />
      <AdminCard key={`admin-${settings.admin.join(',')}`} initial={settings.admin} disabled={!editable} />

      <Card>
        <CardHeader>
          <CardTitle>Network</CardTitle>
          <CardDescription>
            Antennarr listens on a port; how viewers reach it (LAN, VPN, Tailscale) is up to you. Built-in Tailscale is
            planned.
          </CardDescription>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          Access mode:{' '}
          <strong className="text-foreground">{settings.mode === 'open' ? 'open (no viewers)' : 'viewers only'}</strong>
        </CardContent>
      </Card>
    </div>
  )
}

function SaveRow({ children, busy, disabled, error }: { children?: ReactNode; busy: boolean; disabled: boolean; error: string | null }) {
  return (
    <div className="flex items-center gap-3">
      <Button type="submit" disabled={busy || disabled}>
        Save
      </Button>
      {children}
      {error && (
        <span role="alert" className="text-sm text-red-400">
          {error}
        </span>
      )}
    </div>
  )
}

function NumberCard(props: {
  title: string
  description: string
  label: string
  initial: number
  max: number
  disabled: boolean
  field: 'ownerReserve' | 'sharedCap'
  hint: string
}) {
  const { settings } = useEdits()
  const [value, setValue] = useState(String(props.initial))
  const [error, setError] = useState<string | null>(null)
  const id = `setting-${props.field}`

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    const n = Number(value)
    // An empty box reads as 0, which would silently drop the owner reserve.
    if (value.trim() === '' || !Number.isInteger(n) || n < 0) return setError('Enter a whole number, 0 or more.')
    try {
      await settings.mutateAsync({ [props.field]: n } as SettingsPatch)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{props.title}</CardTitle>
        <CardDescription>{props.description}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} noValidate className="space-y-3">
          <div className="space-y-2">
            <Label htmlFor={id}>{props.label}</Label>
            <Input id={id} type="number" min={0} max={props.max} value={value} onChange={(e) => setValue(e.target.value)} className="w-24" disabled={props.disabled} />
            <p className="text-xs text-muted-foreground">{props.hint}</p>
          </div>
          <SaveRow busy={settings.isPending} disabled={props.disabled} error={error} />
        </form>
      </CardContent>
    </Card>
  )
}

function AdminCard({ initial, disabled }: { initial: string[]; disabled: boolean }) {
  const { settings } = useEdits()
  const [text, setText] = useState(initial.join('\n'))
  const [error, setError] = useState<string | null>(null)

  async function save(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await settings.mutateAsync({ admin: parseAddresses(text) })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Admin access</CardTitle>
        <CardDescription>
          Addresses allowed to open this page, besides the machine Antennarr runs on. Viewers are never admins.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={save} noValidate className="space-y-3">
          <div className="space-y-2">
            <Label htmlFor="setting-admin">Admin addresses</Label>
            <Textarea id="setting-admin" value={text} onChange={(e) => setText(e.target.value)} rows={3} className="font-mono" placeholder="100.64.0.0/10" disabled={disabled} />
            <p className="text-xs text-muted-foreground">One per line: an IP or a range. Take care not to lock yourself out.</p>
          </div>
          <SaveRow busy={settings.isPending} disabled={disabled} error={error} />
        </form>
      </CardContent>
    </Card>
  )
}
