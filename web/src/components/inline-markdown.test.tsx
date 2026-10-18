import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { InlineMarkdown } from './inline-markdown'

const show = (text: string) => render(<p data-testid="p"><InlineMarkdown text={text} /></p>)

describe('InlineMarkdown', () => {
  it('draws bold, italic and code', () => {
    show('**Bold** and *italic* and `code` here')
    expect(screen.getByText('Bold').tagName).toBe('STRONG')
    expect(screen.getByText('italic').tagName).toBe('EM')
    expect(screen.getByText('code').tagName).toBe('CODE')
    expect(screen.getByTestId('p')).toHaveTextContent('Bold and italic and code here')
  })
  it('draws something nested, as in a bold line with an italic name inside', () => {
    show('**The firmware *Check now* button works**, and says so')
    const strong = screen.getByText(/The firmware/, { selector: 'strong' })
    expect(strong.querySelector('em')).toHaveTextContent('Check now')
    expect(screen.getByTestId('p')).toHaveTextContent('The firmware Check now button works, and says so')
  })
  it('opens a link in a new tab without handing the page to the other site', () => {
    show('See [the docs](https://example.com/a?b=1) now')
    const a = screen.getByRole('link', { name: 'the docs' })
    expect(a).toHaveAttribute('href', 'https://example.com/a?b=1')
    expect(a).toHaveAttribute('target', '_blank')
    expect(a).toHaveAttribute('rel', 'noopener noreferrer')
  })
  it('does not make a link of anything that is not http or https, and shows just its text', () => {
    show('[click](javascript:void) and [file](file:///etc/passwd) and [data](data:text/html,x) and [rel](/relative)')
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByTestId('p')).toHaveTextContent('click and file and data and rel')
  })
  it('never makes markup of the text: it is all text, whatever it says', () => {
    show('<script>alert(1)</script> <img src=x onerror=alert(1)> **<b>x</b>**')
    expect(document.querySelector('script')).toBeNull()
    expect(document.querySelector('img')).toBeNull()
    expect(screen.getByTestId('p')).toHaveTextContent('<script>alert(1)</script> <img src=x onerror=alert(1)> <b>x</b>')
    expect(screen.getByText('<b>x</b>').tagName).toBe('STRONG')
  })
  it('leaves alone a star that is not emphasis', () => {
    show('2 * 3 = 6 and a * b')
    expect(screen.getByTestId('p')).toHaveTextContent('2 * 3 = 6 and a * b')
    expect(document.querySelector('em')).toBeNull()
  })
  it('leaves unfinished marks as they are', () => {
    show('**not closed and `not closed')
    expect(screen.getByTestId('p')).toHaveTextContent('**not closed and `not closed')
  })
  it('is plain text when there is no Markdown', () => {
    show('Just words.')
    expect(screen.getByTestId('p')).toHaveTextContent('Just words.')
  })
})
