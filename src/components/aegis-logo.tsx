import type { SVGProps } from "react"

/**
 * Aegis brand mark: a protective shield containing an abstract connected
 * Agent network. The geometry is intentionally sparse so it remains legible
 * at navigation-icon sizes.
 */
export function AegisLogo(props: SVGProps<SVGSVGElement>) {
  return (
    <svg
      viewBox="0 0 64 64"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
      {...props}
    >
      <path
        d="M32 5.5 53 13v16.2c0 13.8-8.1 24.2-21 29.3-12.9-5.1-21-15.5-21-29.3V13l21-7.5Z"
        stroke="currentColor"
        strokeWidth="4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="m22 41 10-18 10 18M26 34h12"
        stroke="currentColor"
        strokeWidth="3.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="32" cy="22" r="3.5" fill="currentColor" />
      <circle cx="21.5" cy="42" r="3.5" fill="currentColor" />
      <circle cx="42.5" cy="42" r="3.5" fill="currentColor" />
    </svg>
  )
}
