import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { mockPhone } from '@/test/match-media'

afterEach(() => vi.unstubAllGlobals())

function Demo({ alert = false }: { alert?: boolean }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button onClick={() => setOpen(true)}>Open</button>
      {alert ? (
        <AlertDialog open={open} onOpenChange={setOpen}>
          <AlertDialogContent>
            <AlertDialogTitle>Remove it?</AlertDialogTitle>
            <AlertDialogDescription>This cannot be undone.</AlertDialogDescription>
            <AlertDialogCancel>Keep</AlertDialogCancel>
          </AlertDialogContent>
        </AlertDialog>
      ) : (
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogContent>
            <DialogTitle>Edit it</DialogTitle>
            <DialogDescription>Change things.</DialogDescription>
          </DialogContent>
        </Dialog>
      )}
    </>
  )
}

describe.each([true, false])('dialogs when phone is %s', (phone) => {
  it('opens a dialog, as a bottom sheet only on a phone, and closes with Escape', async () => {
    mockPhone(phone)
    render(<Demo />)
    await userEvent.setup().click(screen.getByRole('button', { name: 'Open' }))
    const d = await screen.findByRole('dialog', { name: 'Edit it' })
    expect(d).toHaveAccessibleDescription('Change things.')
    expect(d.className.includes('bottom-0')).toBe(phone)
    await userEvent.setup().keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('opens an alert dialog and its Cancel button closes it', async () => {
    mockPhone(phone)
    render(<Demo alert />)
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'Open' }))
    const d = await screen.findByRole(phone ? 'dialog' : 'alertdialog', { name: 'Remove it?' })
    expect(d).toHaveAccessibleDescription('This cannot be undone.')
    await user.click(screen.getByRole('button', { name: 'Keep' }))
    await waitFor(() => expect(screen.queryByRole(phone ? 'dialog' : 'alertdialog')).not.toBeInTheDocument())
  })
})
