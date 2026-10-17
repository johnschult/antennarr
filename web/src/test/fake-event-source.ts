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
  readyState = 0 // 0 connecting, 1 open, 2 closed (the browser gave up)
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
    this.readyState = 1
    this.onopen?.()
  }
  /** The connection dropped. With giveUp, the browser won't retry (it got an error reply). */
  fail(giveUp = false) {
    this.readyState = giveUp ? 2 : 0
    this.onerror?.()
  }
  emit(type: string, data: unknown) {
    const e = { data: JSON.stringify(data) } as MessageEvent<string>
    this.listeners.get(type)?.forEach((fn) => fn(e))
  }
}
