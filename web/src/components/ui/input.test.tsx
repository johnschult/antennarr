import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Input } from './input'
import { Textarea } from './textarea'

const asksNotToFill = (el: HTMLElement) => {
  expect(el).toHaveAttribute('autocomplete', 'off')
  expect(el).toHaveAttribute('data-1p-ignore') // 1Password
  expect(el).toHaveAttribute('data-lpignore', 'true') // LastPass
  expect(el).toHaveAttribute('data-bwignore') // Bitwarden
}

describe('fields', () => {
  it('ask password managers and the browser not to fill them', () => {
    render(
      <>
        <Input aria-label="one" />
        <Input aria-label="two" type="number" />
        <Textarea aria-label="three" />
      </>,
    )
    for (const name of ['one', 'two', 'three']) asksNotToFill(screen.getByLabelText(name))
  })

  it('leave it to a field that asks for something else', () => {
    render(<Input aria-label="email" autoComplete="email" />)
    expect(screen.getByLabelText('email')).toHaveAttribute('autocomplete', 'email')
  })
})
