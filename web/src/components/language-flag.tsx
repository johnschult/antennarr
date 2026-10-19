import { CN, DE, ES, FR, IN, IT, JP, KR, PH, PL, PT, RU, SA, US, VN } from 'country-flag-icons/react/3x2'
import { Languages } from 'lucide-react'
import { LANGUAGE_FLAG, languageName } from '@/lib/channel-about'

const FLAGS: Record<string, typeof US> = { CN, DE, ES, FR, IN, IT, JP, KR, PH, PL, PT, RU, SA, US, VN }

/** A language as a flag and its name. A flag only stands for the language to make it quick to see; a language the picture list doesn't have gets a plain mark. */
export function LanguageFlag({ code }: { code: string }) {
  const Flag = FLAGS[LANGUAGE_FLAG[code.toLowerCase()] ?? '']
  return (
    <span className="inline-flex items-center gap-1.5">
      {Flag ? <Flag aria-hidden className="h-3.5 w-5 shrink-0 rounded-[2px] shadow-[0_0_0_1px_rgba(0,0,0,0.15)]" /> : <Languages aria-hidden className="size-4 shrink-0 text-muted-foreground" />}
      {languageName(code)}
    </span>
  )
}
