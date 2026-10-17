import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'

/** Commands to copy, one per line, with a Copy button. */
export function CodeBlock({ lines, label }: { lines: string[]; label: string }) {
  const [copied, setCopied] = useState(false)
  const text = lines.join('\n')

  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error("Couldn't copy: select the text and copy it instead")
    }
  }

  return (
    <div className="relative">
      <pre aria-label={label} className="overflow-x-auto rounded-lg border border-border bg-muted p-4 pr-24 font-mono text-[13px] leading-relaxed">
        {text}
      </pre>
      <Button type="button" size="xs" variant="outline" onClick={copy} className="absolute top-2.5 right-2.5" aria-label={`Copy ${label}`}>
        {copied ? <Check data-icon="inline-start" /> : <Copy data-icon="inline-start" />}
        {copied ? 'Copied' : 'Copy'}
      </Button>
    </div>
  )
}
