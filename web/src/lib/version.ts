// Versions look like v0.2.0, or v0.2.0-3-gabc123 for builds after a tag.
function core(v: string | undefined): [number, number, number] | null {
  const m = /^v?(\d+)\.(\d+)\.(\d+)/.exec(v ?? '')
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null
}

// isBehind reports whether agent is an older release than controller. Unknown
// versions ("dev", empty) are never flagged: better silent than a false alarm.
export function isBehind(agent: string | undefined, controller: string | undefined): boolean {
  const a = core(agent)
  const c = core(controller)
  if (!a || !c) return false
  for (let i = 0; i < 3; i++) if (a[i] !== c[i]) return a[i] < c[i]
  return false
}
