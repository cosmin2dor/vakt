import { useEffect, useState } from 'react'

// Distance from the layout viewport's bottom to the visual viewport's bottom
// — i.e. how much the on-screen keyboard (or browser chrome) currently
// covers. 0 when there's no visualViewport support (falls back to a plain
// sticky/fixed bar with safe-area padding).
export function useVisualViewportBottom(): number {
  const [offset, setOffset] = useState(0)

  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return

    function update() {
      const viewport = window.visualViewport
      if (!viewport) return
      const covered = window.innerHeight - (viewport.height + viewport.offsetTop)
      setOffset(Math.max(0, covered))
    }

    update()
    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
  }, [])

  return offset
}
