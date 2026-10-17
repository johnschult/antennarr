import { useState } from 'react'
import { Switch } from '@/components/ui/switch'
import { Row } from './section'

/** An on/off setting. It takes effect as soon as it is flipped. */
export function SwitchRow({
  id,
  title,
  description,
  checked,
  onChange,
  disabled,
}: {
  id: string
  title: string
  description?: string
  checked: boolean
  onChange: (next: boolean) => Promise<unknown>
  disabled?: boolean
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function flip(next: boolean) {
    setError(null)
    setBusy(true)
    try {
      await onChange(next)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Row title={title} description={description} htmlFor={id} error={error}>
      <Switch id={id} checked={checked} onCheckedChange={flip} disabled={disabled || busy} />
    </Row>
  )
}
