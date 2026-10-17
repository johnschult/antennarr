import { ArrowUpCircle, ExternalLink } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Row, Section } from '@/components/settings/section'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useApp } from '@/hooks/app-context'
import { useEdits } from '@/hooks/mutations'
import { deviceTitle } from '@/lib/device'
import { firmwareDate, isNever, timeAgo } from '@/lib/format'
import type { DeviceState, FirmwareStatus } from '@/lib/types'

export const CHANGELOG_URL = 'https://www.silicondust.com/support/downloads/firmware-changelog.html'

/** A small flag on a tuner that has newer firmware, linking to the details. */
export function FirmwareBadge({ status }: { status: FirmwareStatus }) {
  if (status.state !== 'update-available' && status.state !== 'possible-update') return null
  return (
    <Link to="/tuner" className="inline-flex" title="See what's new">
      <Badge className="gap-1 bg-amber-500/15 text-amber-800 dark:text-amber-300">
        <ArrowUpCircle className="size-3" />
        {status.state === 'update-available' ? 'Firmware update' : 'Firmware: check'}
      </Badge>
    </Link>
  )
}

function StatusBadge({ s }: { s: FirmwareStatus }) {
  switch (s.state) {
    case 'up-to-date':
      return <Badge className="bg-emerald-500/15 text-emerald-800 dark:text-emerald-300">Up to date</Badge>
    case 'update-available':
      return <Badge className="bg-amber-500/15 text-amber-800 dark:text-amber-300">Update available · {firmwareDate(s.latest)}</Badge>
    case 'possible-update':
      return <Badge className="bg-amber-500/15 text-amber-800 dark:text-amber-300">Newer release may apply</Badge>
    case 'disabled':
      return <Badge variant="secondary">Check is off</Badge>
    default:
      return <Badge variant="secondary">Unknown</Badge>
  }
}

/** One tuner's firmware: where it stands, when SiliconDust's changelog was last read, and what's new. */
export function FirmwareSection({ device }: { device: DeviceState }) {
  const { state } = useApp()
  const { firmware: s, tuner } = device
  const { firmware: check } = useEdits()
  const [showAll, setShowAll] = useState(false)
  const several = state.devices.length > 1
  const checking = state.settings.firmwareCheck
  const shown = showAll ? s.updates : s.updates.slice(0, 2)

  return (
    <Section title={several ? `Firmware — ${deviceTitle(device)}` : 'Firmware'}>
      <Row
        title="Version"
        description={`${tuner.firmware.name ?? tuner.model ?? 'Tuner'} · ${s.current || tuner.firmware.version || 'unknown'}`}
      >
        <span className="text-sm font-medium">{firmwareDate(s.current)}</span>
        <StatusBadge s={s} />
      </Row>

      <Row
        title="SiliconDust's changelog"
        description={
          <>
            {isNever(s.checkedAt) ? 'Not read yet.' : `Last read ${timeAgo(s.checkedAt)}.`}
            {s.deviceReported && <> The tuner itself reports {firmwareDate(s.deviceReported)} as available.</>}
            {s.error && <> {capitalise(s.error)}.</>}
          </>
        }
      >
        <Button
          size="sm"
          variant="outline"
          disabled={!checking || check.isPending}
          onClick={() => check.mutate()}
          title={checking ? undefined : 'The firmware check is turned off in Settings'}
        >
          Check now
        </Button>
        <a
          href={CHANGELOG_URL}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
        >
          Changelog <ExternalLink className="size-3" />
        </a>
      </Row>

      {s.updates.length > 0 && (
        <div className="space-y-3 px-4 py-3.5">
          <div className="text-sm font-medium">What's new since {firmwareDate(s.current)}</div>
          <ul className="space-y-3">
            {shown.map((u) => (
              <li key={u.version}>
                <div className="flex items-center gap-2 text-sm font-medium">
                  {firmwareDate(u.version)}
                  {!u.certain && <Badge variant="secondary">may not apply to your model</Badge>}
                </div>
                <ul className="mt-1 list-disc space-y-0.5 pl-5 text-xs text-muted-foreground">
                  {u.notes.map((n, i) => (
                    <li key={i}>
                      {n.scope && <span className="font-medium text-foreground">{n.scope}: </span>}
                      {n.text}
                    </li>
                  ))}
                </ul>
              </li>
            ))}
          </ul>
          {s.updates.length > 2 && (
            <Button size="sm" variant="ghost" onClick={() => setShowAll(!showAll)}>
              {showAll ? 'Show fewer' : `Show all ${s.updates.length} releases`}
            </Button>
          )}
          {!s.modelRecognised && (
            <p className="text-xs text-muted-foreground">Antennarr doesn't recognise this model, so it can't tell which notes are for it; they are all shown.</p>
          )}
          <p className="text-xs text-muted-foreground">Update from the tuner's own page or with SiliconDust's HDHomeRun software; Antennarr never installs firmware.</p>
        </div>
      )}
    </Section>
  )
}

const capitalise = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)
