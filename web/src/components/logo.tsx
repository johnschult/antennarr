/** The Antennarr mark: an antenna on a dark tile. */
export function Logo({ size = 28 }: { size?: number }) {
  return (
    <img
      src={`${import.meta.env.BASE_URL}logo.png`}
      width={size}
      height={size}
      alt=""
      aria-hidden="true"
      className="select-none"
      draggable={false}
    />
  )
}
