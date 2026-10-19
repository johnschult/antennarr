import {
  Baby,
  Briefcase,
  Car,
  ChefHat,
  Church,
  Clapperboard,
  CloudSun,
  Compass,
  Drama,
  Film,
  Gamepad2,
  Ghost,
  GraduationCap,
  HeartPulse,
  Laugh,
  type LucideIcon,
  MessageSquare,
  Music,
  Newspaper,
  Palette,
  Plane,
  Rocket,
  Scale,
  Search,
  ShieldAlert,
  ShoppingBag,
  Tag,
  Trophy,
  Users,
} from 'lucide-react'

/** What a category is shown as: the guide's own word for it, tidied, and a picture. */
interface Look {
  label: string
  icon: LucideIcon
}

const L = (label: string, icon: LucideIcon): Look => ({ label, icon })

/** The guide's categories, by lowercase name. Words that mean the same thing share a label, so they show once. */
const LOOKS: Record<string, Look> = {
  news: L('News', Newspaper),
  newsmagazine: L('News', Newspaper),
  talk: L('Talk', MessageSquare),
  comedy: L('Comedy', Laugh),
  sitcom: L('Comedy', Laugh),
  'comedy drama': L('Comedy', Laugh),
  drama: L('Drama', Drama),
  'crime drama': L('Crime', ShieldAlert),
  crime: L('Crime', ShieldAlert),
  law: L('Law', Scale),
  mystery: L('Mystery', Search),
  documentary: L('Documentary', Clapperboard),
  movie: L('Movies', Film),
  movies: L('Movies', Film),
  sport: L('Sports', Trophy),
  sports: L('Sports', Trophy),
  'sports talk': L('Sports', Trophy),
  football: L('Sports', Trophy),
  children: L('Kids', Baby),
  kids: L('Kids', Baby),
  animated: L('Kids', Baby),
  shop: L('Shopping', ShoppingBag),
  shopping: L('Shopping', ShoppingBag),
  consumer: L('Shopping', ShoppingBag),
  religious: L('Religious', Church),
  community: L('Community', Users),
  reality: L('Reality', Users),
  weather: L('Weather', CloudSun),
  music: L('Music', Music),
  cooking: L('Cooking', ChefHat),
  'how-to': L('How-to', GraduationCap),
  education: L('Education', GraduationCap),
  travel: L('Travel', Plane),
  'bus./financial': L('Business', Briefcase),
  business: L('Business', Briefcase),
  'game show': L('Game shows', Gamepad2),
  'science fiction': L('Sci-fi', Rocket),
  horror: L('Horror', Ghost),
  health: L('Health', HeartPulse),
  auto: L('Auto', Car),
  art: L('Art', Palette),
  action: L('Action', Compass),
  adventure: L('Adventure', Compass),
  western: L('Western', Compass),
}

/** A category as it is shown, with its picture. One the list doesn't know keeps the guide's word and a plain tag. */
export function categoryLook(name: string): Look {
  return LOOKS[name.trim().toLowerCase()] ?? { label: name.trim(), icon: Tag }
}

/** A channel's categories as they are shown: each look once, in the order the guide ranked them. */
export function categoryLooks(names: string[] | undefined): Look[] {
  const seen = new Set<string>()
  const out: Look[] = []
  for (const n of names ?? []) {
    const l = categoryLook(n)
    if (l.label && !seen.has(l.label)) {
      seen.add(l.label)
      out.push(l)
    }
  }
  return out
}

/** The country whose flag stands for a language. A language is not a country, so this is only a picture to make it quick to see. */
export const LANGUAGE_FLAG: Record<string, string> = {
  en: 'US',
  es: 'ES',
  fr: 'FR',
  de: 'DE',
  it: 'IT',
  pt: 'PT',
  zh: 'CN',
  ko: 'KR',
  ja: 'JP',
  vi: 'VN',
  ru: 'RU',
  ar: 'SA',
  hi: 'IN',
  pl: 'PL',
  tl: 'PH',
}

/** A language's name in English: "Spanish" for es. A code it doesn't know is shown as it is. */
export function languageName(code: string): string {
  try {
    return new Intl.DisplayNames(['en'], { type: 'language' }).of(code) ?? code
  } catch {
    return code
  }
}
