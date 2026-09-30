// A short vibration on phones that support it; silently ignored elsewhere.
export function tap() {
  try {
    navigator.vibrate?.(10)
  } catch {
    // Some browsers throw outside a user gesture.
  }
}
