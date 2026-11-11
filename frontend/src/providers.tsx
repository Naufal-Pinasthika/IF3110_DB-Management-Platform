import { useCallback, useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Outlet } from 'react-router'
import { UIContext } from './ui-context'
import type { Confirmation, Notice } from './ui-context'
import { resetSession } from './session'

export function UIProvider({ children }: { children: ReactNode }) {
  const [notice, setNotice] = useState<Notice | null>(null)
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null)
  const resolver = useRef<((value: boolean) => void) | null>(null)
  const dialog = useRef<HTMLDialogElement>(null)
  const cancelButton = useRef<HTMLButtonElement>(null)
  const notify = useCallback((message: string, type: Notice['type'] = 'success') => setNotice({ message, type }), [])
  const confirm = useCallback((options: Confirmation) => new Promise<boolean>(resolve => {
    // A second action cannot take ownership of an already open confirmation.
    if (resolver.current) { resolve(false); return }
    resolver.current = resolve
    setConfirmation(options)
  }), [])
  const settle = (value: boolean) => {
    dialog.current?.close()
    const resolve = resolver.current
    resolver.current = null
    setConfirmation(null)
    resolve?.(value)
  }
  useEffect(() => {
    if (confirmation && !dialog.current?.open) { dialog.current?.showModal(); cancelButton.current?.focus() }
  }, [confirmation])
  useEffect(() => {
    if (notice?.type !== 'success') return
    const timeout = window.setTimeout(() => setNotice(null), 6000)
    return () => window.clearTimeout(timeout)
  }, [notice])
  useEffect(() => () => resolver.current?.(false), [])
  return <UIContext.Provider value={{ notify, confirm }}>
    {children}
    {notice && <div className={`notice global-notice ${notice.type}`} role={notice.type === 'error' ? 'alert' : 'status'}>
      <span id="notice-text">{notice.message}</span>
      <button className="button icon-button" aria-label="Dismiss notification" onClick={() => setNotice(null)}>×</button>
    </div>}
    <dialog ref={dialog} id="confirm-dialog" aria-labelledby="confirm-title" onCancel={event => { event.preventDefault(); settle(false) }}>
      <div className="dialog-heading"><h2 id="confirm-title">{confirmation?.title}</h2></div>
      <p>{confirmation?.description}</p>
      <div className="form-actions">
        <button ref={cancelButton} className="button" onClick={() => settle(false)}>Cancel</button>
        <button className="button danger" onClick={() => settle(true)}>{confirmation?.label || 'Confirm'}</button>
      </div>
    </dialog>
  </UIContext.Provider>
}

export function SessionBoundary() {
  const client = useQueryClient()
  useEffect(() => {
    const expire = () => {
      // Clear all per-user data before rendering another user's session.
      window.dispatchEvent(new Event('discard-changes'))
      void resetSession(client, null)
    }
    window.addEventListener('session-expired', expire)
    return () => window.removeEventListener('session-expired', expire)
  }, [client])
  return <Outlet />
}
