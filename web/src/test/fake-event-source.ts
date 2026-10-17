import { vi } from 'vitest'

type Listener = (e: MessageEvent<string>) => void

/** Just enough of EventSource for the live-state hook, driven by the test. */
export class FakeEventSource {
  static instances: FakeEventSource[] = []
  static last() {
    return FakeEventSource.instances[FakeEventSource.instances.length - 1]
  }
  static install() {
    FakeEventSource.instances = []
    vi.stubGlobal('EventSource', FakeEventSource)
  }

  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  private listeners = new Map<string, Listener[]>()

  readonly url: string

  constructor(url: string) {
    this.url = url
    FakeEventSource.instances.push(this)
  }
  addEventListener(type: string, fn: Listener) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn])
  }
  close() {
    this.closed = true
  }
  // ---- test controls ----
  open() {
    this.onopen?.()
  }
  fail() {
    this.onerror?.()
  }
  emit(type: string, data: unknown) {
    const e = { data: JSON.stringify(data) } as MessageEvent<string>
    this.listeners.get(type)?.forEach((fn) => fn(e))
  }
}
