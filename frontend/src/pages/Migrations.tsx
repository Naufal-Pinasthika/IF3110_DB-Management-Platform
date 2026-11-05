import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router'
import { api, databaseAPI, databaseURL, encode, formatDate } from '../api'
import { useUI } from '../providers'
import type { ExplainResult, Migration, MigrationSummary, SchemaDiff } from '../types'
import { useDatabase } from '../components/layout'
import { ActionButton, Badge, EmptyRow, ErrorState, Loading, PageHeading, useFormAction, useUnsavedChanges } from '../components/ui'
import Plans from '../components/Plans'

export function Migrations() {
  const database = useDatabase()
  const [search, setSearch] = useSearchParams()
  const filter = search.get('status') || ''
  const query = useQuery({
    queryKey: ['migrations', database.id, filter],
    queryFn: ({ signal }) => api<MigrationSummary[]>(`${databaseAPI(database.id)}/migrations${filter ? `?status=${encode(filter)}` : ''}`, { signal }),
    refetchInterval: query => query.state.data?.some(item => item.status === 'RUNNING') ? 3000 : false,
  })
  return <>
    <PageHeading title="Migrations">{database.effective_access === 'VIEW_EDIT' && <Link className="button primary" to={`${databaseURL(database.id)}/migrations/new`}>New migration</Link>}<ActionButton onAction={() => query.refetch()}>Refresh</ActionButton></PageHeading>
    <ErrorState error={query.error} retry={() => query.refetch()} />
    <section className="panel"><div className="panel-toolbar"><div><h2>Migration history</h2><p>Drafts and executed changes for this database.</p></div>
      <label className="toolbar-actions" htmlFor="migration-filter">Status<select id="migration-filter" value={filter} onChange={event => setSearch(event.target.value ? { status: event.target.value } : {})}><option value="">All statuses</option><option value="DRAFT">Draft</option><option value="RUNNING">Running</option><option value="APPLIED">Applied</option><option value="FAILED">Failed</option></select></label>
    </div><div className="table-scroll"><table><thead><tr><th>Migration</th><th>Version</th><th>Status</th><th>Executed by</th><th>Executed at</th></tr></thead><tbody>
      {(query.data || []).map(migration => <tr key={migration.id}><td><Link to={`${databaseURL(database.id)}/migrations/${encode(migration.id)}`}>{migration.name}</Link></td><td>{migration.version ?? '--'}</td><td><Badge value={migration.status} /></td><td>{migration.executed_by || '--'}</td><td>{formatDate(migration.executed_at)}</td></tr>)}
      {!query.data?.length && <EmptyRow columns={5}>{query.isPending ? 'Loading migration history…' : 'No migrations found. Users with edit access can create a draft.'}</EmptyRow>}
    </tbody></table></div></section>
  </>
}

export function MigrationRoute() {
  const { migrationId } = useParams()
  const database = useDatabase()
  const migration = useQuery({
    queryKey: ['migration', database.id, migrationId],
    queryFn: ({ signal }) => api<Migration>(`${databaseAPI(database.id)}/migrations/${encode(migrationId!)}`, { signal }),
    enabled: Boolean(migrationId),
    refetchInterval: query => query.state.data?.status === 'RUNNING' ? 3000 : false,
  })
  if (migrationId && !migration.data) return <><PageHeading title="Migration details" />{migration.isPending ? <Loading>Loading migration…</Loading> : <ErrorState error={migration.error} retry={() => migration.refetch()} />}</>
  return <><ErrorState error={migration.error} retry={() => migration.refetch()} /><MigrationEditor key={migrationId || 'new'} migration={migrationId ? migration.data : undefined} /></>
}

function MigrationEditor({ migration }: { migration?: Migration }) {
  const database = useDatabase()
  const creating = !migration
  const location = useLocation()
  const clone = (location.state as { clone?: { name: string; sql: string } } | null)?.clone
  const [name, setName] = useState(creating ? clone?.name || '' : migration.name)
  const [sql, setSQL] = useState(migration?.sql_text || (creating ? clone?.sql : '') || '')
  const [savedSQL, setSavedSQL] = useState(migration?.sql_text || '')
  const [savedName, setSavedName] = useState('')
  const dirty = sql !== savedSQL || (creating && name !== savedName)
  const matchesSaved = creating || sql === migration.sql_text
  const allowLeave = useUnsavedChanges(dirty)
  const [ddl, setDDL] = useState('')
  const [diff, setDiff] = useState<SchemaDiff | null>(null)
  const [plan, setPlan] = useState<ExplainResult | null>(null)
  const [planSummary, setPlanSummary] = useState('')
  const [note, setNote] = useState('')
  const action = useFormAction()
  const [operation, setOperation] = useState('')
  const navigate = useNavigate()
  const client = useQueryClient()
  const { confirm, notify } = useUI()
  const canEdit = database.effective_access === 'VIEW_EDIT'
  const draft = creating || migration.status === 'DRAFT'
  const editable = canEdit && draft
  const endpoint = `${databaseAPI(database.id)}/migrations${migration ? `/${encode(migration.id)}` : ''}`
  const policy = database.engine === 'mysql' ? 'MySQL: one schema statement per migration (DDL auto-commits), or transactional INSERT/UPDATE/DELETE on InnoDB tables.' : 'All statements execute together in a transaction.'
  const loadedSQL = useRef(migration?.sql_text)
  useEffect(() => {
    if (loadedSQL.current === migration?.sql_text) return
    if (!dirty && migration) {
      loadedSQL.current = migration.sql_text
      setSQL(migration.sql_text)
      setSavedSQL(migration.sql_text)
    }
  }, [migration, dirty])
  const run = (kind: string, work: () => Promise<unknown>) => {
    void action.run(async () => {
      setOperation(kind)
      try { await work() } finally { setOperation('') }
    })
  }
  const changed = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['migrations', database.id] }),
      client.invalidateQueries({ queryKey: ['migration', database.id] }),
      client.invalidateQueries({ queryKey: ['schemas', database.id] }),
      client.invalidateQueries({ queryKey: ['tables', database.id] }),
      client.invalidateQueries({ queryKey: ['table', database.id] }),
      client.invalidateQueries({ queryKey: ['rows', database.id] }),
    ])
  }
  return <>
    <PageHeading title={creating ? 'New migration' : migration.name}>{migration && <ActionButton disabled={action.busy} onAction={changed}>Refresh</ActionButton>}</PageHeading>
    {!creating && <div className="metadata-strip"><div><span>Status</span><strong><Badge value={migration.status} /></strong></div><div><span>Version</span><strong>{migration.version ?? '--'}</strong></div><div><span>Executed by</span><strong>{migration.executed_by || '--'}</strong></div><div><span>Executed at</span><strong>{formatDate(migration.executed_at)}</strong></div></div>}
    {migration?.status === 'RUNNING' && <p className="notice" role="status">This migration is running or awaiting confirmation. Its status refreshes automatically.</p>}
    {migration?.error_summary && <div className="migration-error-card" role="alert"><div className="migration-error-header"><div className="migration-error-title-wrap"><svg className="icon migration-error-icon" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="10" /><path d="M12 8v4m0 4h.01" /></svg><div><strong>Migration execution failed</strong><p className="migration-error-subtitle">{database.engine === 'mysql' ? 'MySQL schema changes auto-commit. Check the target schema and error details before retrying. DML migrations use a transaction.' : 'Review the error details to confirm the execution and rollback outcome before retrying.'}</p></div></div><Badge value="FAILED" /></div><div className="migration-error-body"><div className="migration-error-label">Database Error Output</div><pre className="migration-error-code"><code>{migration.error_summary}</code></pre></div></div>}
    {creating && canEdit && database.engine !== 'mysql' && <section className="panel schema-diff-panel" aria-labelledby="schema-diff-title"><div className="panel-toolbar"><div><h2 id="schema-diff-title">Generate from schema</h2><p>Describe the complete desired schema across all user schemas with CREATE SCHEMA, CREATE TABLE, and CREATE INDEX statements. Unlisted supported objects will be removed; generated SQL may contain DROP statements.</p></div></div>
      <label htmlFor="desired-schema-ddl">Desired schema DDL</label><textarea id="desired-schema-ddl" className="sql-editor schema-ddl-editor" spellCheck={false} disabled={action.busy} value={ddl} onChange={event => setDDL(event.target.value)} placeholder={'CREATE TABLE public.customers (\n  id bigint PRIMARY KEY,\n  email text NOT NULL\n);\n\nCREATE INDEX customers_email_idx ON public.customers (email);'} />
      <div className="editor-footer"><span className="muted" role="status">{operation === 'compare' ? 'Comparing schemas…' : diff ? diff.has_changes ? `${diff.changes.length} changes${diff.changes.some(change => change.action === 'removed') ? ' - candidate includes DROP statements' : ''} - generated SQL is an editable candidate` : 'No supported schema changes found' : 'No comparison run'}</span><button className="button primary" type="button" disabled={action.busy || !ddl.trim()} onClick={() => run('compare', async () => {
        const result = await api<SchemaDiff>(`${databaseAPI(database.id)}/migrations/schema-diff`, { method: 'POST', data: { desired_ddl: ddl } })
        setDiff(result)
        if (!result.has_changes) return
        if (sql.trim() && sql !== result.generated_sql && !await confirm({ title: 'Replace migration SQL?', description: 'The generated candidate will replace the SQL currently in the editor.', label: 'Replace SQL' })) return
        setSQL(result.generated_sql); setPlan(null)
        if (!name.trim()) setName('Generated schema migration')
      })}>Compare schemas</button></div>
      {diff && <SchemaChanges result={diff} />}
    </section>}
    <form id="migration-form" className="panel" aria-busy={action.busy} onSubmit={event => {
      event.preventDefault()
      if (!editable || action.busy) return
      run('save', async () => {
        const result = await api<{ id: string }>(endpoint, { method: creating ? 'POST' : 'PATCH', data: { sql_text: sql, ...(creating ? { name } : {}) } })
        setSavedSQL(sql); setSavedName(name); setNote('Draft saved. Review the SQL before executing.'); setPlan(null)
        if (creating) {
          allowLeave()
          await navigate(`${databaseURL(database.id)}/migrations/${encode(result.id)}`, { replace: true })
        }
        notify('Migration draft saved.')
        await changed()
      })
    }}>
      <div className="panel-toolbar"><div><h2>{creating ? 'Migration draft' : 'SQL changes'}</h2><p>{draft ? 'Saving a draft does not execute SQL.' : 'Executed migrations are read only. Create a new draft for further changes.'}</p></div><div className="toolbar-actions">
        {migration && canEdit && !draft && <Link className="button small" to={`${databaseURL(database.id)}/migrations/new`} state={{ clone: { name: `${migration.name}${migration.status === 'FAILED' ? ' (retry)' : ' (copy)'}`, sql: migration.sql_text } }}>Clone as new draft</Link>}
        <Link className="button small" to={`${databaseURL(database.id)}/migrations`}>Back to migrations</Link>
      </div></div>
      {creating && <div className="editor-name"><label htmlFor="migration-name">Migration name</label><input id="migration-name" required maxLength={200} readOnly={!editable} disabled={action.busy} value={name} onChange={event => { setName(event.target.value); setNote('') }} placeholder="e.g. Add customer email index" /></div>}
      <label className="sr-only" htmlFor="sql">Migration SQL</label><textarea id="sql" name="sql_text" className="sql-editor" spellCheck={false} required readOnly={!editable} disabled={action.busy} value={sql} onChange={event => { setSQL(event.target.value); setPlan(null); setNote('') }} placeholder="-- Write your migration SQL here (e.g. CREATE TABLE items (...);)" />
      <div className="editor-footer"><span className="execution-target">Target: <code>{database.database_name}</code></span><span className="muted" role="status">{operation === 'execute' ? `Executing on ${database.database_name}…` : !draft ? 'Executed SQL is read only.' : dirty ? 'Unsaved changes. Save the draft before executing.' : note || policy}</span><div className="toolbar-actions">
        {editable && <button className="button primary" type="submit" disabled={action.busy}>{operation === 'save' ? 'Saving…' : 'Save draft'}</button>}
        {migration && editable && database.engine !== 'mysql' && <button className="button small" type="button" disabled={action.busy || dirty || !matchesSaved} onClick={() => run('explain', async () => {
          if (dirty || !matchesSaved) throw new Error('Save or refresh the draft before analyzing its plan.')
          const started = performance.now()
          setPlan(await api<ExplainResult>(`${endpoint}/explain`, { method: 'POST', data: { analyze: true } }))
          setPlanSummary(`Rolled back preview - ${((performance.now() - started) / 1000).toFixed(2)}s - actual timings are estimates for this run`)
        })}>{operation === 'explain' ? 'Analyzing…' : 'Analyze plan'}</button>}
        {migration && editable && <button className="button danger" id="execute-migration" type="button" disabled={action.busy || dirty || !matchesSaved} onClick={() => run('execute', async () => {
          if (dirty || !matchesSaved) throw new Error('Save or refresh the draft before executing it.')
          if (!await confirm({ title: 'Execute migration?', description: `Run the saved SQL against ${database.database_name} (${database.label})? ${policy}`, label: 'Execute migration' })) return
          try {
            await api(`${endpoint}/execute`, { method: 'POST' })
            notify('Migration execution completed. Review its status below.')
          } finally { await changed() }
        })}>{operation === 'execute' ? 'Executing…' : 'Execute migration'}</button>}
      </div></div>
      <ErrorState error={action.error} />
    </form>
    {plan && <section className="query-results-surface" aria-label="Migration execution plan"><div className="section-toolbar"><h2>Execution plan</h2><span className="muted" role="status">{planSummary}</span></div><Plans result={plan} /></section>}
    <p className="helper-text">{canEdit ? 'Only drafts can be edited or executed. SELECT queries remain read only.' : 'Read only. Creating or executing migrations requires migration access.'}</p>
  </>
}

function SchemaChanges({ result }: { result: SchemaDiff }) {
  return <div className="schema-diff-results" aria-live="polite">
    {result.excluded_object_count > 0 && <p className="helper-text">{result.excluded_object_count} existing views, sequences, partitions, or other unsupported relations are outside this diff and will not be changed.</p>}
    {!result.has_changes && <p className="inline-state">Supported schema objects match. No migration is needed.</p>}
    {['added', 'modified', 'removed'].map(action => {
      const changes = (result.changes || []).filter(change => change.action === action)
      if (!changes.length) return null
      return <section className="schema-diff-group" key={action}><h3>{action} - {changes.length}</h3>{changes.map((change, index) => <div className={`schema-diff-item ${action}`} key={index}><strong>{change.kind || 'object'} - {change.kind === 'schema' ? change.schema : [change.schema, change.table, change.name !== change.table ? change.name : ''].filter(Boolean).join('.')}</strong>{change.before && <code>Current: {change.before}</code>}{change.after && <code>Desired: {change.after}</code>}</div>)}</section>
    })}
  </div>
}
