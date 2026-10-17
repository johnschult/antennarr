import type { ReactNode } from 'react'

/** `code` inside backticks becomes inline code; everything else stays plain text. */
export function Inline({ text }: { text: string }): ReactNode {
  return text.split(/(`[^`]+`)/g).map((part, i) =>
    part.startsWith('`') && part.endsWith('`') ? (
      <code key={i} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">
        {part.slice(1, -1)}
      </code>
    ) : (
      part
    ),
  )
}

/** A numbered list of steps. */
export function Steps({ steps }: { steps: string[] }) {
  return (
    <ol className="list-decimal space-y-1.5 pl-5 text-sm marker:text-muted-foreground">
      {steps.map((s, i) => (
        <li key={i}>
          <Inline text={s} />
        </li>
      ))}
    </ol>
  )
}
