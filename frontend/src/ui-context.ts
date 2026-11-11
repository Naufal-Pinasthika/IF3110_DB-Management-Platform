import { createContext, useContext } from 'react'

export type Notice = { message: string; type: 'success' | 'error' | '' }
export type Confirmation = { title: string; description: string; label?: string }
export const UIContext = createContext<{
  notify: (message: string, type?: Notice['type']) => void
  confirm: (options: Confirmation) => Promise<boolean>
} | null>(null)

export function useUI() {
  const context = useContext(UIContext)
  if (!context) throw new Error('UIProvider is missing')
  return context
}

