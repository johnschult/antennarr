export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true">
      <rect x="5" y="14" width="22" height="15" rx="3" fill="#0e7490" />
      <rect x="8" y="17" width="16" height="9" rx="1.5" fill="#082f49" />
      <path d="M16 14L9 4M16 14l7-10" stroke="#67e8f9" strokeWidth="2" strokeLinecap="round" />
      <circle cx="9" cy="4" r="2.2" fill="#67e8f9" />
      <circle cx="23" cy="4" r="2.2" fill="#67e8f9" />
    </svg>
  )
}
