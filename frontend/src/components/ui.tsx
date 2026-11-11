import { useEffect, useId, useRef, useState } from 'react'
import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { displayValue, errorMessage } from '../api'
import { useUI } from '../ui-context'
import { useUnsavedChanges } from '../form-hooks'

export function Icon({ name }: { name: string }) {
  const paths: Record<string, ReactNode> = {
    database: <><ellipse cx="12" cy="5" rx="8" ry="3" /><path d="M4 5v7c0 4 16 4 16 0V5M4 12v7c0 4 16 4 16 0v-7" /></>,
    table: <><rect x="3" y="4" width="18" height="16" rx="2" /><path d="M3 9h18M3 14h18M9 9v11" /></>,
    query: <path d="m8 6-6 6 6 6m8-12 6 6-6 6m-2-15-4 18" />,
    history: <path d="M3 11a9 9 0 1 1 3 8M3 4v7h7m2-4v5l3 2" />,
    users: <><circle cx="9" cy="7" r="3" /><path d="M3 21v-3a6 6 0 0 1 12 0v3m2-17a3 3 0 0 1 0 6m2 4a5 5 0 0 1 2 4v3" /></>,
    connect: <path d="m8 3 13 13M4 7l4-4 7 7-4 4zm7 7-3 3H4v4m12-9 4-4m-4-4 4 4" />,
    close: <path d="m6 6 12 12M18 6 6 18" />,
  }
  return <svg className="icon" viewBox="0 0 24 24" aria-hidden="true">{paths[name] || paths.database}</svg>
}

export function PageHeading({ title, children }: { title: string; children?: ReactNode }) {
  useEffect(() => { document.title = `${title} - Database Platform` }, [title])
  return <div className="page-heading"><h1>{title}</h1><div className="heading-actions">{children}</div></div>
}

export function Badge({ value }: { value: string }) {
  const labels: Record<string, string> = { VIEW_ONLY: 'Read only', VIEW_EDIT: 'Read + migrations', DRAFT: 'Draft', RUNNING: 'Running', APPLIED: 'Applied', FAILED: 'Failed', ACTIVE: 'Active', DISABLED: 'Disabled', ENABLED: 'Enabled' }
  const colors: Record<string, string> = { APPLIED: 'green', ACTIVE: 'green', ENABLED: 'green', FAILED: 'red', DISABLED: 'red', RUNNING: 'amber', VIEW_EDIT: 'blue', VIEW_ONLY: 'purple' }
  return <span className={`badge ${colors[value] || ''}`}>{labels[value] || value}</span>
}

export function ErrorState({ error, retry }: { error: unknown; retry?: () => unknown }) {
  if (!error) return null
  return <div className="notice error" role="alert"><span>{errorMessage(error)}</span>{retry && <button type="button" className="button small" onClick={() => retry()}>Try again</button>}</div>
}

export function Loading({ children = 'Loading…' }: { children?: ReactNode }) {
  return <p className="inline-state" role="status">{children}</p>
}

export function EmptyRow({ columns, children = 'No records found.' }: { columns: number; children?: ReactNode }) {
  return <tr><td colSpan={columns} className="empty-cell">{children}</td></tr>
}

export function DataTable({ columns, rows }: { columns: string[]; rows: unknown[][] }) {
  if (!columns?.length) return <Loading>No columns returned.</Loading>
  return <table className="data-table"><thead><tr>{columns.map((column, index) => <th key={`${column}-${index}`} scope="col">{column}</th>)}</tr></thead>
    <tbody>{!rows?.length ? <EmptyRow columns={columns.length}>No rows returned.</EmptyRow> : rows.map((row, index) => <tr key={index}>{columns.map((_, column) => <td key={column}><span className={row[column] === null ? 'null-value' : undefined}>{displayValue(row[column])}</span></td>)}</tr>)}</tbody>
  </table>
}

export function ActionButton({ onAction, children, className = 'button small', disabled, ...props }: Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'onClick'> & { onAction: () => unknown | Promise<unknown> }) {
  const [busy, setBusy] = useState(false)
  const locked = useRef(false)
  const { notify } = useUI()
  return <button {...props} type="button" className={className} disabled={disabled || busy} aria-busy={busy} onClick={async () => {
    if (locked.current) return
    locked.current = true
    setBusy(true)
    try { await onAction() } catch (error) { notify(errorMessage(error), 'error') }
    finally { locked.current = false; setBusy(false) }
  }}>{busy ? 'Working…' : children}</button>
}

export function Modal({ title, children, onClose, busy = false, dirty = false, className = '' }: { title: string; children: ReactNode | ((close: () => void) => ReactNode); onClose: () => void; busy?: boolean; dirty?: boolean; className?: string }) {
  const ref = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  const { confirm } = useUI()
  useUnsavedChanges(dirty)
  useEffect(() => {
    const element = ref.current
    if (!element?.open) element?.showModal()
    return () => { element?.close() }
  }, [])
  const close = async () => {
    if (busy) return
    if (dirty && !await confirm({ title: 'Discard unsaved changes?', description: 'Your changes have not been saved. Discard them and close this dialog?', label: 'Discard changes' })) return
    onClose()
  }
  return <dialog ref={ref} className={className} aria-labelledby={titleId} onCancel={event => { event.preventDefault(); void close() }}>
    <div className="dialog-heading"><h2 id={titleId}>{title}</h2><button type="button" className="button icon-button" aria-label="Close" disabled={busy} onClick={() => void close()}><Icon name="close" /></button></div>
    {typeof children === 'function' ? children(() => { void close() }) : children}
    <button className="sr-only" type="button" onClick={() => void close()}>Close dialog</button>
  </dialog>
}

export function Tabs({ tabs, selected, onSelect, label }: { tabs: { id: string; label: string }[]; selected: string; onSelect: (id: string) => void; label: string }) {
  const id = useId()
  return <div className="tab-bar" role="tablist" aria-label={label}>{tabs.map((tab, index) => <button type="button" className="tab" role="tab" key={tab.id} id={`${id}-${tab.id}`} aria-selected={selected === tab.id} tabIndex={selected === tab.id ? 0 : -1} onClick={() => onSelect(tab.id)} onKeyDown={event => {
    let next: number | undefined
    if (event.key === 'ArrowRight') next = (index + 1) % tabs.length
    if (event.key === 'ArrowLeft') next = (index + tabs.length - 1) % tabs.length
    if (event.key === 'Home') next = 0
    if (event.key === 'End') next = tabs.length - 1
    if (next !== undefined) {
      event.preventDefault(); onSelect(tabs[next].id)
      document.getElementById(`${id}-${tabs[next].id}`)?.focus()
    }
  }}>{tab.label}</button>)}</div>
}
