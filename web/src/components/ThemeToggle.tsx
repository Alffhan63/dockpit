import { Moon, Sun } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/hooks/useTheme'

// ThemeToggle flips between light and dark in one click, like Kashly. Until
// it is first used the theme follows the system setting.
export function ThemeToggle() {
  const { resolved, setTheme } = useTheme()
  const dark = resolved === 'dark'
  const label = dark ? 'Switch to light mode' : 'Switch to dark mode'
  return (
    <Button
      variant="outline"
      size="icon"
      onClick={() => setTheme(dark ? 'light' : 'dark')}
      aria-label={label}
      title={label}
    >
      {dark ? <Sun /> : <Moon />}
    </Button>
  )
}
