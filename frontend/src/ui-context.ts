import { createContext, useContext } from 'react'

// Describe a message that appears after an action
export type Notice = { message: string; type: 'success' | 'error' | '' }

// Describe the question shown before a risky action
export type Confirmation = { title: string; description: string; label?: string }

// Make shared notice and confirmation actions available to the component tree
export const UIContext = createContext<{
  notify: (message: string, type?: Notice['type']) => void
  confirm: (options: Confirmation) => Promise<boolean>
} | null>(null)

// Fail early when a control renders outside the shared provider
export function useUI() {
  const context = useContext(UIContext)
  if (!context) throw new Error('UIProvider is missing')
  return context
}

