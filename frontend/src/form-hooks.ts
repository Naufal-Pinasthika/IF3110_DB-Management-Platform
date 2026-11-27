import { useEffect, useRef, useState } from 'react'
import { useBlocker } from 'react-router'
import { useUI } from './ui-context'

// Track a form action and prevent repeated submits while it is running
export function useFormAction() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const locked = useRef(false)
  const run = async (work: () => Promise<unknown>) => {
    if (locked.current) return
    locked.current = true
    setBusy(true)
    setError(null)

    try {
      await work()
    } catch (failure) {
      setError(failure)
    } finally {
      locked.current = false
      setBusy(false)
    }
  }

  return { busy, error, run }
}

// Protect dirty forms from browser exit and app navigation
export function useUnsavedChanges(dirty: boolean) {
  const { confirm } = useUI()
  const allowed = useRef(false)
  const blocker = useBlocker(() => dirty && !allowed.current)
  // Listen for browser exit and app sign out events
  useEffect(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (dirty && !allowed.current) {
        event.preventDefault()
        event.returnValue = ''
      }
    }

    window.addEventListener('beforeunload', beforeUnload)
    const signout = (event: Event) => {
      if (dirty && !allowed.current) event.preventDefault()
    }
    const discard = () => { allowed.current = true }
    window.addEventListener('request-signout', signout)
    window.addEventListener('discard-changes', discard)
    window.addEventListener('session-expired', discard)
    return () => {
      window.removeEventListener('beforeunload', beforeUnload)
      window.removeEventListener('request-signout', signout)
      window.removeEventListener('discard-changes', discard)
      window.removeEventListener('session-expired', discard)
    }
  }, [dirty])

  // Ask before a route change discards unsaved form data
  useEffect(() => {
    if (blocker.state !== 'blocked') return
    let active = true
    void confirm({
      title: 'Leave without saving?',
      description: 'Your unsaved changes will be lost.',
      label: 'Leave page',
    }).then(leave => {
      if (!active) return
      if (leave) blocker.proceed()
      else blocker.reset()
    })
    return () => { active = false }
  }, [blocker, confirm])

  return () => { allowed.current = true }
}

