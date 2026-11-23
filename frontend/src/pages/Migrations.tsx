import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router'
import { api, databaseAPI, databaseURL, encode, formatDate } from '../api'
import { useUI } from '../ui-context'
import type { ExplainResult, Migration, MigrationExecutionSummary, MigrationSummary, SchemaDiff } from '../types'
import { useDatabase } from '../database-hooks'
import { ActionButton, Badge, EmptyRow, ErrorState, Loading, PageHeading } from '../components/ui'
import { useFormAction, useUnsavedChanges } from '../form-hooks'
import { useSession } from '../session'
import Plans from '../components/Plans'

export function Migrations() {
  const database = useDatabase()
  const [search, setSearch] = useSearchParams()
  const filter = search.get('status') || ''
  const [hideRollback, setHideRollback] = useState(false)

  const migrationsQuery = useQuery({
    queryKey: ['migrations', database.id, filter],
    queryFn: ({ signal }) => api<MigrationSummary[]>(`${databaseAPI(database.id)}/migrations${filter ? `?status=${encode(filter)}` : ''}`, { signal }),
    refetchInterval: query => query.state.data?.some(item => item.status === 'RUNNING' || item.status === 'QUEUED') ? 3000 : false,
  })

  const historyQuery = useQuery({
    queryKey: ['migration-history', database.id, hideRollback],
    queryFn: ({ signal }) => api<MigrationExecutionSummary[]>(`${databaseAPI(database.id)}/migration-history${hideRollback ? '?hide_rollback=true' : ''}`, { signal }),
    refetchInterval: query => query.state.data?.some(item => item.status === 'RUNNING') ? 3000 : false,
  })

  const refreshAll = () => {
    void migrationsQuery.refetch()
    void historyQuery.refetch()
  }

  return <>
    <PageHeading title="Migrations">
      {database.effective_access === 'VIEW_EDIT' && (
        <Link className="button primary" to={`${databaseURL(database.id)}/migrations/new`}>
          New migration
        </Link>
      )}
      <ActionButton onAction={refreshAll}>Refresh</ActionButton>
    </PageHeading>

    <ErrorState error={migrationsQuery.error || historyQuery.error} retry={refreshAll} />

    <section className="panel">
      <div className="panel-toolbar">
        <div>
          <h2>Migration definitions</h2>
          <p>Drafts, approved migrations, and active target schema changes.</p>
        </div>
        <label className="toolbar-actions" htmlFor="migration-filter">
          Status
          <select id="migration-filter" value={filter} onChange={event => setSearch(event.target.value ? { status: event.target.value } : {})}>
            <option value="">All statuses</option>
            <option value="DRAFT">Draft</option>
            <option value="PENDING_REVIEW">Pending review</option>
            <option value="APPROVED">Approved</option>
            <option value="QUEUED">Queued</option>
            <option value="RUNNING">Running</option>
            <option value="APPLIED">Applied</option>
            <option value="FAILED">Failed</option>
            <option value="REJECTED">Rejected</option>
          </select>
        </label>
      </div>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Migration</th>
              <th>Version</th>
              <th>Type</th>
              <th>Status</th>
              <th>Executed by</th>
              <th>Executed at</th>
            </tr>
          </thead>
          <tbody>
            {(migrationsQuery.data || []).map(m => (
              <tr key={m.id}>
                <td>
                  <Link to={`${databaseURL(database.id)}/migrations/${encode(m.id)}`}>{m.name}</Link>
                </td>
                <td>{m.version ?? '--'}</td>
                <td><Badge value={m.migration_type || 'REGULAR'} /></td>
                <td><Badge value={m.status} /></td>
                <td>{m.executed_by || '--'}</td>
                <td>{formatDate(m.executed_at)}</td>
              </tr>
            ))}
            {!migrationsQuery.data?.length && (
              <EmptyRow columns={6}>
                {migrationsQuery.isPending ? 'Loading migration history…' : 'No migrations found. Users with edit access can create a draft.'}
              </EmptyRow>
            )}
          </tbody>
        </table>
      </div>
    </section>

    <section className="panel" style={{ marginTop: '1.5rem' }}>
      <div className="panel-toolbar">
        <div>
          <h2>Execution history</h2>
          <p>Immutable audit trail of all execution attempts (forward, rollback, and failures).</p>
        </div>
        <div className="toolbar-actions" style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
          <label style={{ display: 'inline-flex', alignItems: 'center', gap: '0.4rem', cursor: 'pointer', margin: 0 }}>
            <input
              type="checkbox"
              id="hide-rollback"
              checked={hideRollback}
              onChange={e => setHideRollback(e.target.checked)}
            />
            <span>Hide rollback</span>
          </label>
        </div>
      </div>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Execution ID</th>
              <th>Migration</th>
              <th>Action</th>
              <th>Status</th>
              <th>Batches / Rows</th>
              <th>Executed by</th>
              <th>Started at</th>
            </tr>
          </thead>
          <tbody>
            {(historyQuery.data || []).map(exec => (
              <tr key={exec.id}>
                <td><code>{exec.id.slice(0, 8)}</code></td>
                <td>
                  <Link to={`${databaseURL(database.id)}/migrations/${encode(exec.migration_id)}`}>
                    {exec.migration_name || exec.migration_id.slice(0, 8)}
                  </Link>
                </td>
                <td><Badge value={exec.action_type} /></td>
                <td><Badge value={exec.status} /></td>
                <td>
                  {exec.committed_batches > 0 || exec.committed_rows > 0
                    ? `${exec.committed_batches} batches (${exec.committed_rows} rows)`
                    : '--'}
                </td>
                <td>{exec.executed_by_username || exec.executed_by}</td>
                <td>{formatDate(exec.started_at)}</td>
              </tr>
            ))}
            {!historyQuery.data?.length && (
              <EmptyRow columns={7}>
                {historyQuery.isPending ? 'Loading execution records…' : 'No execution history recorded.'}
              </EmptyRow>
            )}
          </tbody>
        </table>
      </div>
    </section>
  </>
}

export function MigrationRoute() {
  const { migrationId } = useParams()
  const database = useDatabase()
  const migration = useQuery({
    queryKey: ['migration', database.id, migrationId],
    queryFn: ({ signal }) => api<Migration>(`${databaseAPI(database.id)}/migrations/${encode(migrationId!)}`, { signal }),
    enabled: Boolean(migrationId),
    refetchInterval: query =>
      query.state.data?.status === 'RUNNING' || query.state.data?.status === 'QUEUED' ? 3000 : false,
  })

  if (migrationId && !migration.data) {
    return (
      <>
        <PageHeading title="Migration details" />
        {migration.isPending ? <Loading>Loading migration…</Loading> : <ErrorState error={migration.error} retry={() => migration.refetch()} />}
      </>
    )
  }

  return (
    <>
      <ErrorState error={migration.error} retry={() => migration.refetch()} />
      <MigrationEditor key={migrationId || 'new'} migration={migrationId ? migration.data : undefined} />
    </>
  )
}

function MigrationEditor({ migration }: { migration?: Migration }) {
  const database = useDatabase()
  const creating = !migration
  const location = useLocation()
  const clone = (location.state as { clone?: { name: string; sql: string } } | null)?.clone

  const [name, setName] = useState(creating ? clone?.name || '' : migration.name)
  const [sql, setSQL] = useState(migration?.sql_text || (creating ? clone?.sql : '') || '')
  const [sqlDown, setSQLDown] = useState(migration?.sql_down || '')
  const [isIrreversible, setIsIrreversible] = useState(migration?.is_irreversible || false)
  const [irreversibleReason, setIrreversibleReason] = useState(migration?.irreversible_reason || '')
  const [migrationType, setMigrationType] = useState(migration?.migration_type || 'REGULAR')
  const [batchAttribute, setBatchAttribute] = useState(migration?.batch_attribute || 'id')
  const [batchSize, setBatchSize] = useState(migration?.batch_size || 1000)

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
  const session = useSession()
  const currentUser = session.data
  const pendingApproval = migration?.approvals?.find(
    app => app.leader_id === currentUser?.id && app.status === 'PENDING'
  )
  const canDecide = Boolean(pendingApproval)

  const canEdit = database.effective_access === 'VIEW_EDIT'
  const draft = creating || migration.status === 'DRAFT' || migration.status === 'REJECTED'
  const editable = canEdit && draft
  const endpoint = `${databaseAPI(database.id)}/migrations${migration ? `/${encode(migration.id)}` : ''}`
  const policy = database.engine === 'mysql'
    ? 'MySQL: one schema statement per migration (DDL auto-commits), or transactional INSERT/UPDATE/DELETE on InnoDB tables.'
    : 'All statements execute together in a transaction, or step-by-step for batch migrations.'

  const run = (kind: string, work: () => Promise<unknown>) => {
    void action.run(async () => {
      setOperation(kind)
      try {
        await work()
      } finally {
        setOperation('')
      }
    })
  }

  const changed = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['migrations', database.id] }),
      client.invalidateQueries({ queryKey: ['migration', database.id] }),
      client.invalidateQueries({ queryKey: ['migration-history', database.id] }),
      client.invalidateQueries({ queryKey: ['schemas', database.id] }),
      client.invalidateQueries({ queryKey: ['tables', database.id] }),
      client.invalidateQueries({ queryKey: ['table', database.id] }),
      client.invalidateQueries({ queryKey: ['rows', database.id] }),
    ])
  }

  return (
    <>
      <PageHeading title={creating ? 'New migration' : migration.name}>
        {migration && <ActionButton disabled={action.busy} onAction={changed}>Refresh</ActionButton>}
      </PageHeading>

      {!creating && (
        <div className="metadata-strip">
          <div><span>Status</span><strong><Badge value={migration.status} /></strong></div>
          <div><span>Version</span><strong>{migration.version ?? '--'}</strong></div>
          <div><span>Type</span><strong><Badge value={migration.migration_type} /></strong></div>
          <div><span>Reversible</span><strong>{migration.is_irreversible ? 'Irreversible' : 'Reversible'}</strong></div>
          <div><span>Executed at</span><strong>{formatDate(migration.executed_at)}</strong></div>
        </div>
      )}

      {migration?.status === 'RUNNING' && (
        <p className="notice" role="status">
          This migration is executing asynchronously via the worker pool. Status refreshes automatically.
        </p>
      )}

      {migration?.status === 'PENDING_REVIEW' && canDecide && (
        <div className="notice" role="status" style={{ borderLeft: '4px solid #f59e0b' }}>
          <strong>Action Required: Review Pending</strong>
          <p>Your approval is required as group leader before this migration can execute.</p>
        </div>
      )}

      {migration?.status === 'APPROVED' && (
        <div className="notice" role="status" style={{ borderLeft: '4px solid #10b981' }}>
          <strong>Approved & Ready for Execution</strong>
          <p>All required group leaders have approved this migration. It can now be enqueued for worker execution.</p>
        </div>
      )}

      {migration?.status === 'REJECTED' && (
        <div className="notice" role="status" style={{ borderLeft: '4px solid #ef4444' }}>
          <strong>Migration Review Rejected</strong>
          <p>One or more group leaders rejected the requested changes. Update the SQL and resubmit for review.</p>
        </div>
      )}

      {migration?.error_summary && (
        <div className="migration-error-card" role="alert">
          <div className="migration-error-header">
            <div className="migration-error-title-wrap">
              <svg className="icon migration-error-icon" viewBox="0 0 24 24" aria-hidden="true">
                <circle cx="12" cy="12" r="10" />
                <path d="M12 8v4m0 4h.01" />
              </svg>
              <div>
                <strong>Migration execution failed</strong>
                <p className="migration-error-subtitle">Review the error details before retrying or resuming.</p>
              </div>
            </div>
            <Badge value="FAILED" />
          </div>
          <div className="migration-error-body">
            <div className="migration-error-label">Database Error Output</div>
            <pre className="migration-error-code"><code>{migration.error_summary}</code></pre>
          </div>
        </div>
      )}

      {creating && canEdit && database.engine !== 'mysql' && (
        <section className="panel schema-diff-panel" aria-labelledby="schema-diff-title">
          <div className="panel-toolbar">
            <div>
              <h2 id="schema-diff-title">Generate from schema</h2>
              <p>Describe desired schema to generate candidate migration SQL.</p>
            </div>
          </div>
          <label htmlFor="desired-schema-ddl">Desired schema DDL</label>
          <textarea
            id="desired-schema-ddl"
            className="sql-editor schema-ddl-editor"
            spellCheck={false}
            disabled={action.busy}
            value={ddl}
            onChange={event => setDDL(event.target.value)}
            placeholder={'CREATE TABLE public.customers (\n  id bigint PRIMARY KEY,\n  email text NOT NULL\n);'}
          />
          <div className="editor-footer">
            <span className="muted" role="status">
              {operation === 'compare' ? 'Comparing schemas…' : diff ? diff.has_changes ? `${diff.changes.length} changes detected` : 'No supported changes found' : 'No comparison run'}
            </span>
            <button
              className="button primary"
              type="button"
              disabled={action.busy || !ddl.trim()}
              onClick={() => run('compare', async () => {
                const result = await api<SchemaDiff>(`${databaseAPI(database.id)}/migrations/schema-diff`, {
                  method: 'POST',
                  data: { desired_ddl: ddl },
                })
                setDiff(result)
                if (!result.has_changes) return
                if (sql.trim() && sql !== result.generated_sql && !await confirm({
                  title: 'Replace migration SQL?',
                  description: 'The generated candidate will replace the SQL currently in the editor.',
                  label: 'Replace SQL',
                })) return
                setSQL(result.generated_sql)
                setPlan(null)
                if (!name.trim()) setName('Generated schema migration')
              })}
            >
              Compare schemas
            </button>
          </div>
          {diff && <SchemaChanges result={diff} />}
        </section>
      )}

      <form
        id="migration-form"
        className="panel"
        aria-busy={action.busy}
        onSubmit={event => {
          event.preventDefault()
          if (!editable || action.busy) return
          run('save', async () => {
            const data: Record<string, unknown> = {
              sql_text: sql,
              sql_down: isIrreversible ? null : sqlDown,
              is_irreversible: isIrreversible,
              irreversible_reason: isIrreversible ? irreversibleReason : null,
              migration_type: migrationType,
              batch_attribute: migrationType === 'BATCH' ? batchAttribute : null,
              batch_size: migrationType === 'BATCH' ? Number(batchSize) : null,
              ...(creating ? { name } : {}),
            }
            const result = await api<{ id: string }>(endpoint, {
              method: creating ? 'POST' : 'PATCH',
              data,
            })
            setSavedSQL(sql)
            setSavedName(name)
            setNote('Draft saved.')
            setPlan(null)
            if (creating) {
              allowLeave()
              await navigate(`${databaseURL(database.id)}/migrations/${encode(result.id)}`, { replace: true })
            }
            notify('Migration draft saved.')
            await changed()
          })
        }}
      >
        <div className="panel-toolbar">
          <div>
            <h2>{creating ? 'Migration definition' : 'SQL changes'}</h2>
            <p>{draft ? 'Configure migration parameters and statements.' : 'Executed migrations are immutable.'}</p>
          </div>
          <div className="toolbar-actions">
            {migration && canEdit && !draft && (
              <Link
                className="button small"
                to={`${databaseURL(database.id)}/migrations/new`}
                state={{ clone: { name: `${migration.name} (copy)`, sql: migration.sql_text } }}
              >
                Clone as new draft
              </Link>
            )}
            <Link className="button small" to={`${databaseURL(database.id)}/migrations`}>
              Back to migrations
            </Link>
          </div>
        </div>

        {creating && (
          <div className="editor-name">
            <label htmlFor="migration-name">Migration name</label>
            <input
              id="migration-name"
              required
              maxLength={200}
              readOnly={!editable}
              disabled={action.busy}
              value={name}
              onChange={event => { setName(event.target.value); setNote('') }}
              placeholder="e.g. Backfill user analytics"
            />
          </div>
        )}

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '1rem', marginBottom: '1rem' }}>
          <div>
            <label htmlFor="migration-type">Mode Eksekusi</label>
            <select
              id="migration-type"
              disabled={!editable || action.busy}
              value={migrationType}
              onChange={e => setMigrationType(e.target.value)}
            >
              <option value="REGULAR">Regular (single transaction)</option>
              <option value="BATCH">Batch (cursor-based incremental)</option>
            </select>
          </div>

          {migrationType === 'BATCH' && (
            <>
              <div>
                <label htmlFor="batch-attr">Batch Attribute (Cursor Column)</label>
                <input
                  id="batch-attr"
                  disabled={!editable || action.busy}
                  value={batchAttribute}
                  onChange={e => setBatchAttribute(e.target.value)}
                  placeholder="e.g. id"
                />
              </div>
              <div>
                <label htmlFor="batch-size">Batch Size</label>
                <input
                  id="batch-size"
                  type="number"
                  min={1}
                  max={50000}
                  disabled={!editable || action.busy}
                  value={batchSize}
                  onChange={e => setBatchSize(Number(e.target.value))}
                />
              </div>
            </>
          )}
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'inline-flex', alignItems: 'center', gap: '0.5rem', cursor: editable ? 'pointer' : 'default' }}>
            <input
              type="checkbox"
              disabled={!editable || action.busy}
              checked={isIrreversible}
              onChange={e => setIsIrreversible(e.target.checked)}
            />
            <strong>Irreversible migration (no rollback possible)</strong>
          </label>
          {isIrreversible && (
            <div style={{ marginTop: '0.5rem' }}>
              <label htmlFor="irreversible-reason">Irreversible Reason</label>
              <input
                id="irreversible-reason"
                required
                disabled={!editable || action.busy}
                value={irreversibleReason}
                onChange={e => setIrreversibleReason(e.target.value)}
                placeholder="Explain why this migration cannot be safely undone (e.g. destructive data purge)"
              />
            </div>
          )}
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <label htmlFor="sql">SQL Up (Execute forward)</label>
          <textarea
            id="sql"
            name="sql_text"
            className="sql-editor"
            spellCheck={false}
            required
            readOnly={!editable}
            disabled={action.busy}
            value={sql}
            onChange={event => { setSQL(event.target.value); setPlan(null); setNote('') }}
            placeholder={
              migrationType === 'BATCH'
                ? 'UPDATE users SET status = 1 WHERE id IN (SELECT id FROM users WHERE id > {{CURSOR}} ORDER BY id ASC LIMIT {{BATCH_SIZE}});'
                : 'CREATE TABLE example (...);'
            }
          />
        </div>

        {!isIrreversible && (
          <div style={{ marginBottom: '1rem' }}>
            <label htmlFor="sql-down">SQL Down (Rollback / Batch revert)</label>
            <textarea
              id="sql-down"
              name="sql_down"
              className="sql-editor"
              spellCheck={false}
              readOnly={!editable}
              disabled={action.busy}
              value={sqlDown}
              onChange={event => setSQLDown(event.target.value)}
              placeholder={
                migrationType === 'BATCH'
                  ? 'UPDATE users SET status = 0 WHERE id IN (SELECT id FROM users WHERE id <= {{CURSOR}} ORDER BY id DESC LIMIT {{BATCH_SIZE}});'
                  : 'DROP TABLE IF EXISTS example;'
              }
            />
          </div>
        )}

        <div className="editor-footer">
          <span className="execution-target">Target: <code>{database.database_name}</code></span>
          <span className="muted" role="status">
            {operation === 'execute' ? `Queuing execution…` : note || policy}
          </span>
          <div className="toolbar-actions" style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            {editable && (
              <button className="button primary" type="submit" disabled={action.busy}>
                {operation === 'save' ? 'Saving…' : 'Save draft'}
              </button>
            )}

            {migration && editable && database.engine !== 'mysql' && (
              <button
                className="button small"
                type="button"
                disabled={action.busy || dirty || !matchesSaved}
                onClick={() => run('explain', async () => {
                  if (dirty || !matchesSaved) throw new Error('Save or refresh the draft before analyzing its plan.')
                  const started = performance.now()
                  setPlan(await api<ExplainResult>(`${endpoint}/explain`, { method: 'POST', data: { analyze: true } }))
                  setPlanSummary(`Rolled back preview - ${((performance.now() - started) / 1000).toFixed(2)}s - actual timings are estimates for this run`)
                })}
              >
                {operation === 'explain' ? 'Analyzing…' : 'Analyze plan'}
              </button>
            )}

            {migration && editable && (migration.status === 'DRAFT' || migration.status === 'REJECTED') && (
              <button
                className="button secondary"
                type="button"
                disabled={action.busy || dirty || !matchesSaved}
                onClick={() => run('submit-review', async () => {
                  if (dirty || !matchesSaved) throw new Error('Save changes before submitting for review.')
                  await api(`${endpoint}/submit-review`, { method: 'POST' })
                  notify('Submitted for review. Awaiting group leader approvals.')
                  await changed()
                })}
              >
                {operation === 'submit-review' ? 'Submitting…' : 'Submit for review'}
              </button>
            )}

            {migration && migration.status === 'PENDING_REVIEW' && canDecide && (
              <>
                <button
                  className="button primary"
                  type="button"
                  disabled={action.busy}
                  onClick={() => run('approve', async () => {
                    if (!await confirm({
                      title: 'Approve migration?',
                      description: 'Confirm that your group approves this migration for execution.',
                      label: 'Approve',
                    })) return
                    await api(`${endpoint}/approve`, { method: 'POST', data: {} })
                    notify('Approval recorded.')
                    await changed()
                  })}
                >
                  {operation === 'approve' ? 'Approving…' : 'Approve'}
                </button>
                <button
                  className="button danger"
                  type="button"
                  disabled={action.busy}
                  onClick={() => run('reject', async () => {
                    if (!await confirm({
                      title: 'Reject migration?',
                      description: 'Are you sure you want to reject this migration? It will return to draft for changes.',
                      label: 'Reject',
                    })) return
                    await api(`${endpoint}/reject`, { method: 'POST', data: {} })
                    notify('Migration rejected.')
                    await changed()
                  })}
                >
                  {operation === 'reject' ? 'Rejecting…' : 'Reject'}
                </button>
              </>
            )}

            {migration && migration.status === 'APPROVED' && canEdit && (
              <button
                className="button danger"
                id="execute-migration"
                type="button"
                disabled={action.busy || dirty}
                onClick={() => run('execute', async () => {
                  if (!await confirm({
                    title: 'Execute migration?',
                    description: `Queue this migration to run against ${database.database_name}? Execution is asynchronous.`,
                    label: 'Execute migration',
                  })) return
                  await api(`${endpoint}/execute`, { method: 'POST' })
                  notify('Migration queued for asynchronous execution.')
                  await changed()
                })}
              >
                {operation === 'execute' ? 'Queuing…' : 'Execute migration'}
              </button>
            )}

            {migration && migration.can_rollback && canEdit && (
              <button
                className="button danger"
                type="button"
                disabled={action.busy}
                onClick={() => run('rollback', async () => {
                  if (!await confirm({
                    title: 'Rollback migration?',
                    description: `Roll back ${migration.name} using its approved SQL down?`,
                    label: 'Rollback',
                  })) return
                  await api(`${endpoint}/rollback`, { method: 'POST' })
                  notify('Rollback job queued.')
                  await changed()
                })}
              >
                {operation === 'rollback' ? 'Rolling back…' : 'Rollback migration'}
              </button>
            )}

            {migration && migration.can_forward && canEdit && (
              <button
                className="button secondary"
                type="button"
                disabled={action.busy}
                onClick={() => run('forward', async () => {
                  if (!await confirm({
                    title: 'Forward migration?',
                    description: `Re-apply ${migration.name} forward?`,
                    label: 'Forward',
                  })) return
                  await api(`${endpoint}/forward`, { method: 'POST' })
                  notify('Forward execution queued.')
                  await changed()
                })}
              >
                {operation === 'forward' ? 'Forwarding…' : 'Forward migration'}
              </button>
            )}
          </div>
        </div>
        <ErrorState error={action.error} />
      </form>

      {migration?.approvals && migration.approvals.length > 0 && (
        <section className="panel" style={{ marginTop: '1.5rem' }}>
          <div className="panel-toolbar">
            <div>
              <h2>Group Approvals</h2>
              <p>Each team with access to this database must have its leader approve before execution.</p>
            </div>
          </div>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Group</th>
                  <th>Assigned Leader</th>
                  <th>Status</th>
                  <th>Decision Time</th>
                  <th>Review Comment</th>
                </tr>
              </thead>
              <tbody>
                {migration.approvals.map(app => (
                  <tr key={app.id}>
                    <td><strong>{app.group_name}</strong></td>
                    <td>{app.leader_username}</td>
                    <td><Badge value={app.status} /></td>
                    <td>{formatDate(app.decided_at)}</td>
                    <td>{app.comment || '--'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {plan && (
        <section className="query-results-surface" aria-label="Migration execution plan">
          <div className="section-toolbar">
            <h2>Execution plan</h2>
            <span className="muted" role="status">{planSummary}</span>
          </div>
          <Plans result={plan} />
        </section>
      )}
    </>
  )
}

function SchemaChanges({ result }: { result: SchemaDiff }) {
  return (
    <div className="schema-diff-results" aria-live="polite">
      {result.excluded_object_count > 0 && (
        <p className="helper-text">
          {result.excluded_object_count} existing objects are outside this diff and will not be changed.
        </p>
      )}
      {!result.has_changes && <p className="inline-state">Supported schema objects match. No migration is needed.</p>}
      {['added', 'modified', 'removed'].map(action => {
        const changes = (result.changes || []).filter(change => change.action === action)
        if (!changes.length) return null
        return (
          <section className="schema-diff-group" key={action}>
            <h3>{action} - {changes.length}</h3>
            {changes.map((change, index) => (
              <div className={`schema-diff-item ${action}`} key={index}>
                <strong>
                  {change.kind || 'object'} -{' '}
                  {change.kind === 'schema'
                    ? change.schema
                    : [change.schema, change.table, change.name !== change.table ? change.name : ''].filter(Boolean).join('.')}
                </strong>
                {change.before && <code>Current: {change.before}</code>}
                {change.after && <code>Desired: {change.after}</code>}
              </div>
            ))}
          </section>
        )
      })}
    </div>
  )
}
