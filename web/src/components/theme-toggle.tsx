import { Monitor, Moon, Sun } from 'lucide-react'
import type { ComponentType } from 'react'
import { useTheme } from '@/hooks/theme-context'
import type { ThemePreference } from '@/lib/theme'
import { cn } from '@/lib/utils'

const options: { value: ThemePreference; label: string; icon: ComponentType<{ className?: string }> }[] = [
  { value: 'light', label: 'Light', icon: Sun },
  { value: 'dark', label: 'Dark', icon: Moon },
  { value: 'system', label: 'System', icon: Monitor },
]

/** Light / dark / follow the system: one button each, the current one pressed. */
export function ThemeToggle() {
  const { preference, setPreference } = useTheme()
  return (
    <div role="group" aria-label="Theme" className="inline-flex rounded-full border border-border bg-card p-0.5">
      {options.map(({ value, label, icon: Icon }) => (
        <button
          key={value}
          type="button"
          aria-label={label}
          aria-pressed={preference === value}
          title={label}
          onClick={() => setPreference(value)}
          className={cn(
            'grid size-7 place-items-center rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
            preference === value ? 'bg-foreground text-background' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          <Icon className="size-3.5" />
        </button>
      ))}
    </div>
  )
}
