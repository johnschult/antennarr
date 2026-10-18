import type { ReactNode } from 'react'

/**
 * Draws the little Markdown the changelog uses inside a line: **bold**, *italic*, `code` and
 * [links](https://…). Everything is built as React elements, never as HTML, so nothing in the text can add
 * markup; and a link only becomes a link if it is http or https, otherwise just its text is shown.
 */
const INLINE = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)|\*([^*\s][^*]*?)\*/

const safeLink = (href: string) => /^https?:\/\//i.test(href)

function render(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = []
  let rest = text
  let n = 0
  while (rest.length > 0) {
    const m = INLINE.exec(rest)
    if (!m) {
      out.push(rest)
      break
    }
    if (m.index > 0) out.push(rest.slice(0, m.index))
    const k = `${key}.${n++}`
    if (m[1] !== undefined) out.push(<strong key={k}>{render(m[1], k)}</strong>)
    else if (m[2] !== undefined) out.push(<code key={k} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">{m[2]}</code>)
    else if (m[3] !== undefined) {
      out.push(
        safeLink(m[4]) ? (
          <a key={k} href={m[4]} target="_blank" rel="noopener noreferrer" className="underline underline-offset-2 hover:text-foreground">
            {render(m[3], k)}
          </a>
        ) : (
          <span key={k}>{render(m[3], k)}</span>
        ),
      )
    } else out.push(<em key={k}>{render(m[5], k)}</em>)
    rest = rest.slice(m.index + m[0].length)
  }
  return out
}

export function InlineMarkdown({ text }: { text: string }) {
  return <>{render(text, 'm')}</>
}
