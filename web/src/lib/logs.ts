import { Bug, CircleX, Info, TriangleAlert, type LucideIcon } from 'lucide-react'
import type { LogEntry, LogLevel } from '@/lib/types'

export const LEVELS: LogLevel[] = ['debug', 'info', 'warn', 'error']

/** How a level is shown: its name, a picture and a color. */
export const LEVEL_LOOK: Record<LogLevel, { label: string; icon: LucideIcon; tone: string }> = {
  debug: { label: 'Debug', icon: Bug, tone: 'text-muted-foreground' },
  info: { label: 'Info', icon: Info, tone: 'text-cyan-700 dark:text-cyan-300' },
  warn: { label: 'Warning', icon: TriangleAlert, tone: 'text-amber-700 dark:text-amber-300' },
  error: { label: 'Error', icon: CircleX, tone: 'text-red-700 dark:text-red-300' },
}

/** The clock time of a line, to the millisecond. */
export function clock(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.toLocaleTimeString([], { hour12: false })}.${String(d.getMilliseconds()).padStart(3, '0')}`
}

/** A line as one line of text, the way it would be in a file: time, level, message, then key=value. */
export function lineText(e: LogEntry): string {
  const attrs = (e.attrs ?? []).map((a) => `${a.key}=${/[\s"=]/.test(a.value) ? JSON.stringify(a.value) : a.value}`)
  return [e.time, e.level.toUpperCase().padEnd(5), e.message, ...attrs].join(' ')
}
