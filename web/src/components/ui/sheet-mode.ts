import * as React from "react"
import { useIsPhone } from "@/hooks/use-media-query"

const SheetMode = React.createContext<boolean | null>(null)

/**
 * Whether a dialog is a sheet (on a phone) or a box in the middle. Decided when it opens and kept until it
 * closes, so turning a phone across the breakpoint doesn't swap one for the other mid-use and reset what is in it.
 */
export function useSheetChoice(open?: boolean) {
  const phone = useIsPhone()
  const [sheet, setSheet] = React.useState(phone)
  if (!open && sheet !== phone) setSheet(phone)
  return sheet
}

/** The parts of a dialog ask this which of the two it is, so they all agree with the root. */
export function useSheet() {
  const chosen = React.useContext(SheetMode)
  const phone = useIsPhone()
  return chosen ?? phone
}

export const SheetModeProvider = SheetMode.Provider

