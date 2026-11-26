import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { Icon, PageHeading, Tabs } from '../components/ui'
import { MigrationDefinition } from '../components/migrations/MigrationDefinition'
import { MigrationReview } from '../components/migrations/MigrationReview'
import { MigrationActionDialog, MigrationHistory, MigrationQueue } from '../components/migrations/MigrationExecutions'
import { emptyDraft } from '../components/migrations/types'
import type { MigrationDraft } from '../components/migrations/types'
import { useDatabase } from '../database-hooks'
import { databaseURL } from '../api'
import '../components/migrations/migrations.css'

const tabs = [
  { id: 'draft', label: 'Migration draft' },
  { id: 'review', label: 'Review & approval' },
  { id: 'queue', label: 'Queue & progress' },
  { id: 'history', label: 'Execution history' },
]
const steps = ['Draft', 'Pending review', 'Approved', 'Queued', 'Running', 'Success / Failed']

export function MigrationWorkspace({ databaseName = 'No database selected' }: { databaseName?: string }) {
  const [search, setSearch] = useSearchParams()
  const tab = tabs.some(item => item.id === search.get('tab')) ? search.get('tab')! : 'draft'
  const [draft, setDraft] = useState<MigrationDraft>({ ...emptyDraft })
  const [savedDraft, setSavedDraft] = useState<MigrationDraft | null>(null)
  const [action, setAction] = useState<'rollback' | 'forward' | null>(null)
  const saved = savedDraft !== null && JSON.stringify(savedDraft) === JSON.stringify(draft)
  const selectTab = (id: string) => setSearch(previous => { const next = new URLSearchParams(previous); next.set('tab', id); return next })
  return <div className="m2-workspace">
    <PageHeading title="Migration workflow"><button type="button" className="button" onClick={() => setAction('rollback')}>Request rollback</button><button type="button" className="button" onClick={() => setAction('forward')}>Request forward</button></PageHeading>
    <p className="notice m2-preview-notice"><span className="badge blue">Preview</span><span>Prepare and explore the migration workflow. Changes stay on this page; execution and approvals are not available yet.</span></p>
    <div className="metadata-strip"><div><span>Target database</span><strong className="mono">{databaseName}</strong></div><div><span>Migration</span><strong>{draft.name || 'Untitled draft'}</strong></div><div><span>Draft state</span><strong aria-live="polite">{saved ? 'Saved locally' : savedDraft ? 'Unsaved changes' : 'New draft'}</strong></div></div>
    <ol className="m2-workflow-steps" aria-label="Migration lifecycle">{steps.map((step, index) => <li key={step} aria-current={index === 0 ? 'step' : undefined}><span aria-hidden="true">{index + 1}</span>{step}</li>)}</ol>
    <Tabs tabs={tabs} label="Migration workspace" selected={tab} onSelect={selectTab} />
    <section role="tabpanel" aria-label="Migration draft" hidden={tab !== 'draft'}><MigrationDefinition value={draft} onChange={setDraft} onSave={() => setSavedDraft({ ...draft })} /></section>
    <section role="tabpanel" aria-label="Review and approval" hidden={tab !== 'review'}><MigrationReview /></section>
    <section role="tabpanel" aria-label="Queue and progress" hidden={tab !== 'queue'}><MigrationQueue /></section>
    <section role="tabpanel" aria-label="Execution history" hidden={tab !== 'history'}><MigrationHistory /></section>
    {action && <MigrationActionDialog action={action} onClose={() => setAction(null)} />}
  </div>
}

export function DatabaseMigrationWorkspace() {
  const database = useDatabase()
  return <><p className="helper-text"><Link to={`${databaseURL(database.id)}/migrations`}>← Back to migrations</Link></p><MigrationWorkspace databaseName={database.database_name} /></>
}

// A public, isolated preview: no session boundary, database queries, or API calls.
export function MigrationWorkspacePreview() {
  return <>
    <a className="skip-link" href="#main">Skip to content</a>
    <div className="app-shell m2-preview-shell">
      <aside className="sidebar" aria-label="Preview navigation"><Link className="brand" to="/preview/milestone-2"><Icon name="database" /><span>Database Platform</span></Link><nav className="main-nav" aria-label="Preview"><Link to="/preview/milestone-2" aria-current="page"><Icon name="history" />Migration workflow</Link><Link to="/databases"><Icon name="database" />Back to application</Link></nav></aside>
      <div className="workspace"><header className="topbar"><div className="connection-context"><Icon name="database" /><strong>Migration workspace</strong></div><span className="badge blue">Milestone 2 preview</span></header><main id="main" className="content"><MigrationWorkspace /></main></div>
    </div>
  </>
}
