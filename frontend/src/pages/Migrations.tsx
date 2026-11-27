import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router'
import { api, databaseAPI, databaseURL, encode, formatDate } from '../api'
import { useUI } from '../ui-context'
import type { ExplainResult, JobPage, Migration, MigrationExecutionSummary, MigrationSummary, SchemaDiff } from '../types'
import { useDatabase } from '../database-hooks'
import { ActionButton, Badge, EmptyRow, ErrorState, Loading, Modal, PageHeading } from '../components/ui'
import { useFormAction, useUnsavedChanges } from '../form-hooks'
import { useSession } from '../session'
import { ExecutionProgress } from '../components/migrations/MigrationExecutions'
import Plans from '../components/Plans'

// List migration definitions and execution records for the selected database
export function Migrations() {
  const database = useDatabase()
  const [search, setSearch] = useSearchParams()
  const filter = search.get('status') || ''
  const [hideRollback, setHideRollback] = useState(false)
  const [selectedExecId, setSelectedExecId] = useState<string | null>(null)
  const recoveryAction = useFormAction()
  const { confirm, notify } = useUI()
  const canEdit = database.effective_access === 'VIEW_EDIT'

  // Refresh definition status only while queued work is active
  const migrationsQuery = useQuery({
    queryKey: ['migrations', database.id, filter],
    queryFn: ({ signal }) =>
      api<MigrationSummary[]>(
        `${databaseAPI(database.id)}/migrations${filter ? `?status=${encode(filter)}` : ''}`,
        { signal },
      ),
    refetchInterval: query =>
      query.state.data?.some(
        item => item.status === 'RUNNING' || item.status === 'QUEUED',
      )
        ? 3000
        : false,
  })

  // Show authoritative queue state, attempts, and worker lease information.
  const jobsQuery = useQuery({
    queryKey: ['jobs', database.id],
    queryFn: ({ signal }) =>
      api<JobPage>(`${databaseAPI(database.id)}/jobs?limit=50`, { signal }),
    refetchInterval: query =>
      query.state.data?.jobs.some(job =>
        ['PENDING', 'RUNNING', 'RETRY_WAIT'].includes(job.status),
      )
        ? 2000
        : false,
  })

  // Refresh history while an execution is still running
  const historyQuery = useQuery({
    queryKey: ['migration-history', database.id, hideRollback],
    queryFn: ({ signal }) =>
      api<MigrationExecutionSummary[]>(
        `${databaseAPI(database.id)}/migration-history${hideRollback ? '?hide_rollback=true' : ''}`,
        { signal },
      ),
    refetchInterval: query =>
      query.state.data?.some(item =>
        item.status === 'RUNNING' ||
        (item.status === 'UNKNOWN' && ['PENDING', 'RUNNING', 'RETRY_WAIT'].includes(item.job_status || '')),
      )
        ? 3000
        : false,
  })

  // Refresh both lists after a user requests a manual update
  const refreshAll = () => {
    void migrationsQuery.refetch()
    void historyQuery.refetch()
    void jobsQuery.refetch()
  }

  const handleResumeExecution = (executionId: string) => {
    void recoveryAction.run(async () => {
      if (!await confirm({
        title: 'Resume batch execution?',
        description: 'Continue processing remaining uncommitted batches from the last saved checkpoint?',
        label: 'Resume from checkpoint',
      })) return
      await api(`${databaseAPI(database.id)}/executions/${encode(executionId)}/resume`, { method: 'POST' })
      notify('Resume job queued from checkpoint.')
      refreshAll()
    })
  }

  const handleCancelBatch = (executionId: string) => {
    void recoveryAction.run(async () => {
      if (!await confirm({
        title: 'Cancel batch and compensate?',
        description: 'Run approved SQL down batch-by-batch in reverse order to compensate committed batches?',
        label: 'Request batch compensation',
      })) return
      await api(`${databaseAPI(database.id)}/executions/${encode(executionId)}/cancel-batch`, { method: 'POST' })
      notify('Batch compensation job queued.')
      refreshAll()
    })
  }

  const handleReconcileExecution = (executionId: string, outcome: 'APPLIED' | 'NOT_APPLIED') => {
    void recoveryAction.run(async () => {
      const applied = outcome === 'APPLIED'
      if (!await confirm({
        title: applied ? 'Confirm execution applied?' : 'Confirm execution not applied?',
        description: applied
          ? 'Only choose this after inspecting the target and confirming all intended changes are present. Do not use this for a partial result.'
          : 'Only choose this after inspecting the target and confirming none of this execution’s changes took effect. Do not use this for a partial result.',
        label: applied ? 'Confirm applied' : 'Confirm not applied',
      })) return
      await api(`${databaseAPI(database.id)}/executions/${encode(executionId)}/reconcile`, {
        method: 'POST',
        data: { outcome },
      })
      notify(applied ? 'Execution reconciled as applied.' : 'Execution reconciled as not applied.')
      refreshAll()
    })
  }

  const selectedExecSummary = (historyQuery.data || []).find(item => item.id === selectedExecId)
  const selectedExecutionProp = selectedExecSummary ? {
    id: selectedExecSummary.id,
    migration: selectedExecSummary.migration_name || selectedExecSummary.migration_id.slice(0, 8),
    revision: selectedExecSummary.version ? `v${selectedExecSummary.version}` : '--',
    action: selectedExecSummary.action_type,
    actor: selectedExecSummary.executed_by_username || selectedExecSummary.executed_by,
    status: selectedExecSummary.status as 'PENDING' | 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'PARTIAL' | 'PARTIALLY_REVERTED' | 'UNKNOWN',
    startedAt: selectedExecSummary.started_at,
    sql: selectedExecSummary.executed_sql,
    rolledBackExecutionId: selectedExecSummary.rolled_back_execution_id || undefined,
    committedBatches: selectedExecSummary.committed_batches,
    committedRows: selectedExecSummary.committed_rows,
    cursor: selectedExecSummary.last_cursor_value || undefined,
    attempt: 1,
  } : undefined

  return <>
    <PageHeading title="Migrations">
      {canEdit && (
        <Link className="button primary" to={`${databaseURL(database.id)}/migrations/new`}>
          New migration
        </Link>
      )}
      <ActionButton onAction={refreshAll}>Refresh</ActionButton>
    </PageHeading>

    <ErrorState error={migrationsQuery.error || historyQuery.error || jobsQuery.error || recoveryAction.error} retry={refreshAll} />

    <section className="panel">
      <div className="panel-toolbar">
        <div>
          <h2>Migration definitions</h2>
        </div>
        <label className="toolbar-actions" htmlFor="migration-filter">
          Status
          <select
            id="migration-filter"
            value={filter}
            onChange={event =>
              setSearch(event.target.value ? { status: event.target.value } : {})
            }
          >
            <option value="">All statuses</option>
            <option value="DRAFT">Draft</option>
            <option value="PENDING_REVIEW">Pending review</option>
            <option value="APPROVED">Approved</option>
            <option value="ROLLBACK_APPROVED">Rollback approved</option>
            <option value="ROLLED_BACK">Rolled back</option>
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
                {migrationsQuery.isPending
                  ? 'Loading migration history…'
                  : 'No migrations found'}
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
        </div>
        <div
          className="toolbar-actions"
          style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}
        >
          <label
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '0.4rem',
              cursor: 'pointer',
              margin: 0,
            }}
          >
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
              <th>Cursor</th>
              <th>Executed by</th>
              <th>Started at</th>
              <th>Recovery</th>
            </tr>
          </thead>
          <tbody>
            {(historyQuery.data || []).map(exec => {
              const canResumeRow = canEdit && (exec.status === 'PARTIAL' || exec.status === 'PARTIALLY_REVERTED')
              const canCancelRow = canEdit && exec.status === 'PARTIAL' && (exec.action_type === 'FORWARD' || exec.action_type === 'RETRY')
              const canReconcileRow = canEdit && exec.status === 'UNKNOWN' && exec.job_status === 'BLOCKED'
              return (
                <tr key={exec.id}>
                  <td><code>{exec.id.slice(0, 8)}</code></td>
                  <td>
                    <Link to={`${databaseURL(database.id)}/migrations/${encode(exec.migration_id)}`}>
                      {exec.migration_name || exec.migration_id.slice(0, 8)}
                    </Link>
                  </td>
                  <td><Badge value={exec.action_type} /></td>
                  <td>
                    <Badge value={exec.status} />
                    {exec.job_status && <span className="cell-subtitle">Job: {exec.job_status.toLowerCase().replaceAll('_', ' ')}</span>}
                  </td>
                  <td>
                    {exec.committed_batches > 0 || exec.committed_rows > 0
                      ? `${exec.committed_batches} batches (${exec.committed_rows} rows)`
                      : '--'}
                  </td>
                  <td><code>{exec.last_cursor_value || '--'}</code></td>
                  <td>{exec.executed_by_username || exec.executed_by}</td>
                  <td>{formatDate(exec.started_at)}</td>
                  <td>
                    <div style={{ display: 'flex', gap: '0.4rem', flexWrap: 'wrap' }}>
                      <button
                        type="button"
                        className="button small"
                        onClick={() => setSelectedExecId(selectedExecId === exec.id ? null : exec.id)}
                      >
                        {selectedExecId === exec.id ? 'Hide' : 'Details'}
                      </button>
                      {canResumeRow && (
                        <button
                          type="button"
                          className="button small primary"
                          disabled={recoveryAction.busy}
                          onClick={() => handleResumeExecution(exec.id)}
                        >
                          Resume
                        </button>
                      )}
                      {canCancelRow && (
                        <button
                          type="button"
                          className="button small danger"
                          disabled={recoveryAction.busy}
                          onClick={() => handleCancelBatch(exec.id)}
                        >
                          Compensate
                        </button>
                      )}
                      {canReconcileRow && (
                        <>
                          <button
                            type="button"
                            className="button small primary"
                            disabled={recoveryAction.busy}
                            onClick={() => handleReconcileExecution(exec.id, 'APPLIED')}
                          >
                            Confirm applied
                          </button>
                          <button
                            type="button"
                            className="button small danger"
                            disabled={recoveryAction.busy}
                            onClick={() => handleReconcileExecution(exec.id, 'NOT_APPLIED')}
                          >
                            Confirm not applied
                          </button>
                        </>
                      )}
                    </div>
                  </td>
                </tr>
              )
            })}
            {!historyQuery.data?.length && (
              <EmptyRow columns={9}>
                {historyQuery.isPending
                  ? 'Loading execution records…'
                  : 'No execution history recorded.'}
              </EmptyRow>
            )}
          </tbody>
        </table>
      </div>
    </section>

    <section className="panel" style={{ marginTop: '1.5rem' }}>
      <div className="panel-toolbar">
        <div>
          <h2>Worker queue</h2>
        </div>
        <ActionButton onAction={() => jobsQuery.refetch()} disabled={jobsQuery.isFetching}>
          Refresh queue
        </ActionButton>
      </div>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Job</th>
              <th>Status</th>
              <th>Attempt</th>
              <th>Progress</th>
              <th>Lease expires</th>
              <th>Next attempt</th>
              <th>State</th>
            </tr>
          </thead>
          <tbody>
            {(jobsQuery.data?.jobs || []).map(job => (
              <tr key={job.id}>
                <td><code>{job.id.slice(0, 8)}</code></td>
                <td>
                  <Badge value={job.status} />
                  {job.database_blocked && <span className="cell-subtitle">Database blocked</span>}
                </td>
                <td>{job.attempt_count} / {job.max_attempts}</td>
                <td>{job.committed_batches} batches · {job.committed_rows.toLocaleString()} rows</td>
                <td>{formatDate(job.lease_expires_at)}</td>
                <td>{formatDate(job.next_attempt_at)}</td>
                <td>
                  {job.result?.state ? <Badge value={job.result.state} /> : job.error_code || '--'}
                  {job.needs_inspection && <span className="cell-subtitle">Inspection required</span>}
                </td>
              </tr>
            ))}
            {!jobsQuery.data?.jobs.length && (
              <EmptyRow columns={7}>
                {jobsQuery.isPending ? 'Loading worker queue…' : 'No worker jobs recorded for this database.'}
              </EmptyRow>
            )}
          </tbody>
        </table>
      </div>
      {jobsQuery.data?.next_before && (
        <p className="helper-text">Showing the 50 most recent jobs. Older job state remains available in execution history.</p>
      )}
    </section>

    {selectedExecutionProp && (
      <ExecutionProgress
        execution={selectedExecutionProp}
        canEdit={canEdit}
        busy={recoveryAction.busy}
        onResume={handleResumeExecution}
        onCancelBatch={handleCancelBatch}
      />
    )}
  </>
}

// Load one migration when the route includes its id
export function MigrationRoute() {
  const { migrationId } = useParams()
  const database = useDatabase()
  const migration = useQuery({
    queryKey: ['migration', database.id, migrationId],
    queryFn: ({ signal }) =>
      api<Migration>(
        `${databaseAPI(database.id)}/migrations/${encode(migrationId!)}`,
        { signal },
      ),
    enabled: Boolean(migrationId),
    refetchInterval: query =>
      query.state.data?.status === 'RUNNING' || query.state.data?.status === 'QUEUED' ? 3000 : false,
  })

  if (migrationId && !migration.data) {
    return (
      <>
        <PageHeading title="Migration details" />
        {migration.isPending ? (
          <Loading>Loading migration…</Loading>
        ) : (
          <ErrorState error={migration.error} retry={() => migration.refetch()} />
        )}
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

// Edit drafts and show actions allowed by the current migration state
function MigrationEditor({ migration }: { migration?: Migration }) {
  const database = useDatabase()
  const creating = !migration
  const location = useLocation()
  // Read clone data from the route state only when creating a new draft
  const clone = (location.state as { clone?: { name: string; sql: string } } | null)?.clone

  const [name, setName] = useState(creating ? clone?.name || '' : migration.name)
  const [sql, setSQL] = useState(migration?.sql_text || (creating ? clone?.sql : '') || '')
  const [sqlDown, setSQLDown] = useState(migration?.sql_down || '')
  const [isIrreversible, setIsIrreversible] = useState(migration?.is_irreversible || false)
  const [irreversibleReason, setIrreversibleReason] = useState(migration?.irreversible_reason || '')
  const [migrationType, setMigrationType] = useState(migration?.migration_type || 'REGULAR')
  const [batchAttribute, setBatchAttribute] = useState(migration?.batch_attribute || 'id')
  const [batchSize, setBatchSize] = useState(migration?.batch_size || 1000)

  // Keep saved values separate from fields the user is still editing
  const [savedSQL, setSavedSQL] = useState(migration?.sql_text || '')
  const [savedName, setSavedName] = useState('')
  // Treat changed SQL and a changed draft name as unsaved work
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
  // Only the assigned leader can decide this pending approval
  const pendingApproval = migration?.approvals?.find(
    app => app.leader_id === currentUser?.id && app.status === 'PENDING',
  )
  const canDecide = Boolean(pendingApproval)
  const reviewAction = migration?.approvals?.[0]?.action || 'MIGRATE'
  const reviewLabel =
    reviewAction === 'ROLLBACK' ? 'rollback' : reviewAction === 'FORWARD' ? 'forward migration' : 'migration'
  const [decisionDialog, setDecisionDialog] = useState<'approve' | 'reject' | null>(null)
  const [reviewComment, setReviewComment] = useState('')

  const migrationHistoryQuery = useQuery({
    queryKey: ['migration-history', database.id, 'detail', migration?.id],
    queryFn: ({ signal }) => api<MigrationExecutionSummary[]>(`${databaseAPI(database.id)}/migration-history`, { signal }),
    enabled: Boolean(migration?.id),
    refetchInterval: query =>
      query.state.data?.some(item =>
        item.migration_id === migration?.id &&
        (item.status === 'RUNNING' ||
          (item.status === 'UNKNOWN' && ['PENDING', 'RUNNING', 'RETRY_WAIT'].includes(item.job_status || ''))),
      )
        ? 3000
        : false,
  })
  const migrationExecutions = (migrationHistoryQuery.data || []).filter(item => item.migration_id === migration?.id)
  const partialExecutions = migrationExecutions.filter(
    item => item.status === 'PARTIAL' || item.status === 'PARTIALLY_REVERTED'
  )
  const unknownExecutions = migrationExecutions.filter(
    item => item.status === 'UNKNOWN' && item.job_status === 'BLOCKED',
  )

  const canEdit = database.effective_access === 'VIEW_EDIT'
  const draft = creating || migration.status === 'DRAFT' || migration.status === 'REJECTED'
  const editable = canEdit && draft
  const endpoint = `${databaseAPI(database.id)}/migrations${migration ? `/${encode(migration.id)}` : ''}`
  const policy =
    database.engine === 'mysql'
      ? 'MySQL: one schema statement per migration (DDL auto-commits), or transactional INSERT/UPDATE/DELETE on InnoDB tables.'
      : ''

  // Track the active action and always clear its label after it settles
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

  // Refresh every database view that can change after migration work
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

  const reconcileExecution = (executionId: string, outcome: 'APPLIED' | 'NOT_APPLIED') => {
    run(`reconcile-${outcome}`, async () => {
      const applied = outcome === 'APPLIED'
      if (!await confirm({
        title: applied ? 'Confirm execution applied?' : 'Confirm execution not applied?',
        description: applied
          ? 'Inspect the target and confirm all intended changes are present. Do not use this for a partial result.'
          : 'Inspect the target and confirm none of this execution’s changes took effect. Do not use this for a partial result.',
        label: applied ? 'Confirm applied' : 'Confirm not applied',
      })) return
      await api(`${databaseAPI(database.id)}/executions/${encode(executionId)}/reconcile`, {
        method: 'POST',
        data: { outcome },
      })
      notify(applied ? 'Execution reconciled as applied.' : 'Execution reconciled as not applied.')
      await changed()
    })
  }

  // Render state notices first then the editor and allowed actions
  return (
    <>
      <PageHeading title={creating ? 'New migration' : migration.name}>
        {migration && <ActionButton disabled={action.busy} onAction={changed}>Refresh</ActionButton>}
      </PageHeading>

      {!creating && (
        <div className="metadata-strip">
          <div>
            <span>Status</span>
            <strong><Badge value={migration.status} /></strong>
          </div>
          <div>
            <span>Version</span>
            <strong>{migration.version ?? '--'}</strong>
          </div>
          <div>
            <span>Type</span>
            <strong><Badge value={migration.migration_type} /></strong>
          </div>
          <div>
            <span>Reversible</span>
            <strong>{migration.is_irreversible ? 'Irreversible' : 'Reversible'}</strong>
          </div>
          <div>
            <span>Executed at</span>
            <strong>{formatDate(migration.executed_at)}</strong>
          </div>
        </div>
      )}

      {migration?.status === 'RUNNING' && (
        <p className="notice" role="status">
          This migration is executing asynchronously via the worker pool. Status refreshes automatically.
        </p>
      )}

      {migration?.status === 'PENDING_REVIEW' && canDecide && (
        <div className="notice" role="status" style={{ borderLeft: '4px solid #f59e0b' }}>
          <strong>Action Required: {reviewAction === 'ROLLBACK' ? 'Rollback Review Pending' : reviewAction === 'FORWARD' ? 'Forward Review Pending' : 'Review Pending'}</strong>
          <p>Your approval is required as group leader before this {reviewLabel} can execute.</p>
        </div>
      )}

      {migration?.status === 'APPROVED' && (
        <div className="notice" role="status" style={{ borderLeft: '4px solid #10b981' }}>
          <strong>Approved & Ready for Execution</strong>
          <p>All required group leaders have approved this migration. It can now be enqueued for worker execution.</p>
        </div>
      )}

      {migration?.status === 'ROLLBACK_APPROVED' && (
        <div className="notice" role="status" style={{ borderLeft: '4px solid #f59e0b' }}>
          <strong>Rollback Approved & Ready for Execution</strong>
          <p>All required group leaders have approved rolling back this migration. You can now enqueue the rollback job.</p>
        </div>
      )}

      {migration?.status === 'ROLLED_BACK' && (
        <div className="notice" role="status" style={{ borderLeft: '4px solid #6366f1' }}>
          <strong>Migration Rolled Back</strong>
          <p>This migration has been rolled back. Re-applying it forward requires submitting a forward review for group leader approval.</p>
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
              {operation === 'compare'
                ? 'Comparing schemas…'
                : diff
                  ? diff.has_changes
                    ? `${diff.changes.length} changes detected`
                    : 'No supported changes found'
                  : 'No comparison run'}
            </span>
            <button
              className="button primary"
              type="button"
              disabled={action.busy || !ddl.trim()}
              onClick={() =>
                run('compare', async () => {
                  // Keep the generated diff visible even when no SQL is produced
                  const result = await api<SchemaDiff>(
                    `${databaseAPI(database.id)}/migrations/schema-diff`,
                    {
                      method: 'POST',
                      data: { desired_ddl: ddl },
                    },
                  )
                  setDiff(result)
                  if (!result.has_changes) return

                  // Ask before replacing SQL already entered by the user
                  if (
                    sql.trim() &&
                    sql !== result.generated_sql &&
                    !(await confirm({
                      title: 'Replace migration SQL?',
                      description:
                        'The generated candidate will replace the SQL currently in the editor.',
                      label: 'Replace SQL',
                    }))
                  ) {
                    return
                  }

                  setSQL(result.generated_sql)
                  setPlan(null)
                  if (!name.trim()) setName('Generated schema migration')
                })
              }
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

          // Send only fields supported by the selected migration mode
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
              await navigate(
                `${databaseURL(database.id)}/migrations/${encode(result.id)}`,
                { replace: true },
              )
            }
            notify('Migration draft saved.')
            await changed()
          })
        }}
      >
        <div className="panel-toolbar">
          <div>
            <h2>{creating ? 'Migration definition' : 'SQL changes'}</h2>
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
              onChange={event => {
                setName(event.target.value)
                setNote('')
              }}
              placeholder="e.g. Backfill user analytics"
            />
          </div>
        )}

        <div className="migration-parameters">
          <div>
            <label htmlFor="migration-type">Execution mode</label>
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
          <label
            style={{
              display: 'inline-flex',
              alignItems: 'center',
              gap: '0.5rem',
              cursor: editable ? 'pointer' : 'default',
            }}
          >
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
          {(operation === 'execute' || note || policy) && (
            <span className="muted" role="status">
              {operation === 'execute' ? 'Queuing execution…' : note || policy}
            </span>
          )}
          <div className="toolbar-actions" style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            {editable && (
              <button className="button primary" type="submit" disabled={action.busy}>
                {operation === 'save' ? 'Saving…' : 'Save draft'}
              </button>
            )}

            {migration && editable && migration.status === 'DRAFT' && database.engine !== 'mysql' && (
              <button
                className="button small"
                type="button"
                disabled={action.busy || dirty || !matchesSaved}
                onClick={() =>
                  run('explain', async () => {
                    // Plan analysis runs against the saved migration on the server
                    if (dirty || !matchesSaved) {
                      throw new Error(
                        'Save or refresh the draft before analyzing its plan.',
                      )
                    }
                    const started = performance.now()
                    setPlan(
                      await api<ExplainResult>(`${endpoint}/explain`, {
                        method: 'POST',
                        data: { analyze: true },
                      }),
                    )
                    setPlanSummary(
                      `Rolled back preview - ${((performance.now() - started) / 1000).toFixed(2)}s - actual timings are estimates for this run`,
                    )
                  })
                }
              >
                {operation === 'explain' ? 'Analyzing…' : 'Analyze plan'}
              </button>
            )}

            {migration && editable && (migration.status === 'DRAFT' || migration.status === 'REJECTED') && (
              <button
                className="button secondary"
                type="button"
                disabled={action.busy || dirty || !matchesSaved}
                onClick={() =>
                  run('submit-review', async () => {
                    // Review requests must use the saved SQL version
                    if (dirty || !matchesSaved) {
                      throw new Error('Save changes before submitting for review.')
                    }
                    await api(`${endpoint}/submit-review`, { method: 'POST' })
                    notify('Submitted for review. Awaiting group leader approvals.')
                    await changed()
                  })
                }
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
                  onClick={() => {
                    setReviewComment('')
                    setDecisionDialog('approve')
                  }}
                >
                  Approve
                </button>
                <button
                  className="button danger"
                  type="button"
                  disabled={action.busy}
                  onClick={() => {
                    setReviewComment('')
                    setDecisionDialog('reject')
                  }}
                >
                  Reject
                </button>
              </>
            )}

            {migration && migration.status === 'APPROVED' && canEdit && (
              <button
                className="button danger"
                id="execute-migration"
                type="button"
                disabled={action.busy || dirty}
                onClick={() =>
                  run('execute', async () => {
                    // Execution is queued only after the user confirms the target
                    if (!(await confirm({
                      title: 'Execute migration?',
                      description: `Queue this migration to run against ${database.database_name}? Execution is asynchronous.`,
                      label: 'Execute migration',
                    }))) {
                      return
                    }
                    await api(`${endpoint}/execute`, { method: 'POST' })
                    notify('Migration queued for asynchronous execution.')
                    await changed()
                  })
                }
              >
                {operation === 'execute' ? 'Queuing…' : 'Execute migration'}
              </button>
            )}

            {migration && migration.can_request_rollback && canEdit && (
              <button
                className="button secondary"
                type="button"
                disabled={action.busy}
                onClick={() => run('submit-rollback-review', async () => {
                  if (!await confirm({
                    title: 'Request rollback review?',
                    description: `Submit rollback of ${migration.name} for group leader approval? All required leaders must approve before rollback executes.`,
                    label: 'Request rollback review',
                  })) return
                  await api(`${endpoint}/submit-rollback-review`, { method: 'POST' })
                  notify('Rollback review submitted. Awaiting group leader approvals.')
                  await changed()
                })}
              >
                {operation === 'submit-rollback-review' ? 'Submitting…' : 'Request rollback review'}
              </button>
            )}

            {migration && migration.can_rollback && canEdit && (
              <button
                className="button danger"
                type="button"
                disabled={action.busy}
                onClick={() =>
                  run('rollback', async () => {
                    // Roll back only after confirming the approved down SQL
                    if (!(await confirm({
                      title: 'Execute approved rollback?',
                      description: `Roll back ${migration.name} using its approved SQL down? Execution is asynchronous.`,
                      label: 'Execute rollback',
                    }))) {
                      return
                    }
                    await api(`${endpoint}/rollback`, { method: 'POST' })
                    notify('Rollback job queued.')
                    await changed()
                  })
                }
              >
                {operation === 'rollback' ? 'Rolling back…' : 'Execute rollback'}
              </button>
            )}

            {migration && migration.can_request_forward && canEdit && (
              <button
                className="button secondary"
                type="button"
                disabled={action.busy}
                onClick={() => run('submit-forward-review', async () => {
                  if (!await confirm({
                    title: 'Request forward review?',
                    description: `Submit ${migration.name} for group leader approval before re-applying forward?`,
                    label: 'Request forward review',
                  })) return
                  await api(`${endpoint}/submit-forward-review`, { method: 'POST' })
                  notify('Forward review submitted. Awaiting group leader approvals.')
                  await changed()
                })}
              >
                {operation === 'submit-forward-review' ? 'Submitting…' : 'Request forward review'}
              </button>
            )}

            {migration && migration.can_forward && canEdit && (
              <button
                className="button secondary"
                type="button"
                disabled={action.busy}
                onClick={() =>
                  run('forward', async () => {
                    // Forward execution reapplies the migration after confirmation
                    if (!(await confirm({
                      title: 'Forward migration?',
                      description: `Re-apply ${migration.name} forward?`,
                      label: 'Forward',
                    }))) {
                      return
                    }
                    await api(`${endpoint}/forward`, { method: 'POST' })
                    notify('Forward execution queued.')
                    await changed()
                  })
                }
              >
                {operation === 'forward' ? 'Forwarding…' : 'Forward migration'}
              </button>
            )}
          </div>
        </div>
        <ErrorState error={action.error} />
      </form>

      {migration && partialExecutions.length > 0 && (
        <section className="panel" style={{ marginTop: '1.5rem' }}>
          <div className="panel-toolbar">
            <div>
              <h2>Batch Checkpoint Recovery</h2>
              <p>Resume from the last committed checkpoint or run reverse batch compensation.</p>
            </div>
          </div>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Execution</th>
                  <th>Action</th>
                  <th>Status</th>
                  <th>Committed Batches / Rows</th>
                  <th>Last Cursor</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {partialExecutions.map(exec => {
                  const canResumeExec = canEdit && (exec.status === 'PARTIAL' || exec.status === 'PARTIALLY_REVERTED')
                  const canCancelExec =
                    canEdit &&
                    exec.status === 'PARTIAL' &&
                    (exec.action_type === 'FORWARD' || exec.action_type === 'RETRY') &&
                    !migration.is_irreversible &&
                    Boolean(migration.sql_down)
                  return (
                    <tr key={exec.id}>
                      <td><code>{exec.id.slice(0, 8)}</code></td>
                      <td><Badge value={exec.action_type} /></td>
                      <td><Badge value={exec.status} /></td>
                      <td>{exec.committed_batches} batches ({exec.committed_rows} rows)</td>
                      <td><code>{exec.last_cursor_value || '--'}</code></td>
                      <td>
                        <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                          {canResumeExec && (
                            <button
                              type="button"
                              className="button small primary"
                              disabled={action.busy}
                              onClick={() => run('resume', async () => {
                                if (!await confirm({
                                  title: 'Resume batch execution?',
                                  description: 'Continue processing remaining batches from the last committed checkpoint?',
                                  label: 'Resume from checkpoint',
                                })) return
                                await api(`${databaseAPI(database.id)}/executions/${encode(exec.id)}/resume`, { method: 'POST' })
                                notify('Resume job queued from checkpoint.')
                                await changed()
                              })}
                            >
                              {operation === 'resume' ? 'Resuming…' : 'Resume from checkpoint'}
                            </button>
                          )}
                          {canCancelExec && (
                            <button
                              type="button"
                              className="button small danger"
                              disabled={action.busy}
                              onClick={() => run('cancel-batch', async () => {
                                if (!await confirm({
                                  title: 'Cancel batch and compensate?',
                                  description: 'Execute approved SQL down batch-by-batch in reverse order to compensate committed rows?',
                                  label: 'Request batch compensation',
                                })) return
                                await api(`${databaseAPI(database.id)}/executions/${encode(exec.id)}/cancel-batch`, { method: 'POST' })
                                notify('Batch compensation job queued.')
                                await changed()
                              })}
                            >
                              {operation === 'cancel-batch' ? 'Cancelling…' : 'Cancel batch (compensate)'}
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          {partialExecutions.some(e => e.error_summary) && (
            <div style={{ marginTop: '0.75rem' }}>
              <strong>Last checkpoint error</strong>
              <pre className="migration-error-code"><code>{partialExecutions.find(e => e.error_summary)?.error_summary}</code></pre>
            </div>
          )}
        </section>
      )}

      {migration && unknownExecutions.length > 0 && (
        <section className="panel" style={{ marginTop: '1.5rem' }}>
          <div className="panel-toolbar">
            <div>
              <h2>Execution outcome requires inspection</h2>
              <p>Inspect the target before confirming whether an unknown execution applied.</p>
            </div>
          </div>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Execution</th>
                  <th>Action</th>
                  <th>Worker job</th>
                  <th>Error</th>
                  <th>Reconcile</th>
                </tr>
              </thead>
              <tbody>
                {unknownExecutions.map(exec => (
                  <tr key={exec.id}>
                    <td><code>{exec.id}</code></td>
                    <td><Badge value={exec.action_type} /></td>
                    <td><Badge value={exec.job_status || 'UNKNOWN'} /></td>
                    <td>{exec.error_summary || 'Outcome not confirmed'}</td>
                    <td>
                      {canEdit ? (
                        <div className="toolbar-actions">
                          <button
                            type="button"
                            className="button small primary"
                            disabled={action.busy}
                            onClick={() => reconcileExecution(exec.id, 'APPLIED')}
                          >
                            Confirm applied
                          </button>
                          <button
                            type="button"
                            className="button small danger"
                            disabled={action.busy}
                            onClick={() => reconcileExecution(exec.id, 'NOT_APPLIED')}
                          >
                            Confirm not applied
                          </button>
                        </div>
                      ) : 'Requires migration access'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {migration?.approvals && migration.approvals.length > 0 && (
        <section className="panel" style={{ marginTop: '1.5rem' }}>
          <div className="panel-toolbar">
            <div>
              <h2>Group Approvals</h2>
            </div>
          </div>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Action</th>
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
                    <td><Badge value={app.action || 'MIGRATE'} /></td>
                    <td><strong>{app.group_name}</strong></td>
                    <td>{app.leader_username || 'Unassigned (awaiting leader)'}</td>
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

      {decisionDialog && (
        <Modal
          title={decisionDialog === 'approve' ? `Approve ${reviewLabel}` : `Reject ${reviewLabel}`}
          onClose={() => setDecisionDialog(null)}
          busy={action.busy}
        >
          {close => (
            <form
              aria-busy={action.busy}
              onSubmit={event => {
                event.preventDefault()
                void action.run(async () => {
                  // Send the selected decision with an optional trimmed comment
                  const endpointPath = `${endpoint}/${decisionDialog}`
                  const payload: { comment?: string } = {}
                  if (reviewComment.trim()) {
                    payload.comment = reviewComment.trim()
                  }
                  await api(endpointPath, {
                    method: 'POST',
                    data: payload,
                  })
                  notify(decisionDialog === 'approve' ? 'Approval recorded.' : 'Review rejected.')
                  setDecisionDialog(null)
                  await changed()
                })
              }}
            >
              <fieldset className="form-fields" disabled={action.busy}>
                <p className="muted">
                  {decisionDialog === 'approve'
                    ? `Confirm that your group approves this ${reviewLabel} for "${migration?.name}".`
                    : reviewAction === 'ROLLBACK'
                    ? `Reject rollback for "${migration?.name}". The migration will remain applied.`
                    : reviewAction === 'FORWARD'
                    ? `Reject forward re-execution for "${migration?.name}". It will remain rolled back.`
                    : `Reject "${migration?.name}". It will be returned to draft state for revisions.`}
                </p>
                <label htmlFor="review-comment">
                  Review comment {decisionDialog === 'reject' ? '(Recommended)' : '(Optional)'}
                </label>
                <textarea
                  id="review-comment"
                  rows={3}
                  autoFocus
                  placeholder={
                    decisionDialog === 'approve'
                      ? 'e.g. Schemas and queries look good for our services.'
                      : 'e.g. Please add rollback SQL or revise table constraints.'
                  }
                  value={reviewComment}
                  onChange={e => setReviewComment(e.target.value)}
                />
              </fieldset>
              <ErrorState error={action.error} />
              <div className="form-actions">
                <button className="button" type="button" disabled={action.busy} onClick={close}>
                  Cancel
                </button>
                <button
                  className={`button ${decisionDialog === 'approve' ? 'primary' : 'danger'}`}
                  type="submit"
                  disabled={action.busy}
                >
                  {action.busy
                    ? decisionDialog === 'approve'
                      ? 'Approving…'
                      : 'Rejecting…'
                    : decisionDialog === 'approve'
                    ? `Approve ${reviewLabel}`
                    : `Reject ${reviewLabel}`}
                </button>
              </div>
            </form>
          )}
        </Modal>
      )}
    </>
  )
}

// Group generated schema changes so each action is easy to scan
function SchemaChanges({ result }: { result: SchemaDiff }) {
  return (
    <div className="schema-diff-results" aria-live="polite">
      {result.excluded_object_count > 0 && (
        <p className="helper-text">
          {result.excluded_object_count} existing objects are outside this diff and will not be changed.
        </p>
      )}
      {!result.has_changes && (
        <p className="inline-state">
          Supported schema objects match. No migration is needed.
        </p>
      )}
      {['added', 'modified', 'removed'].map(action => {
        const changes = (result.changes || []).filter(change => change.action === action)
        if (!changes.length) return null
        return (
          <section className="schema-diff-group" key={action}>
            <h3>
              {action} - {changes.length}
            </h3>
            {changes.map((change, index) => (
              <div className={`schema-diff-item ${action}`} key={index}>
                <strong>
                  {change.kind || 'object'} -{' '}
                  {change.kind === 'schema'
                    ? change.schema
                    : [
                        change.schema,
                        change.table,
                        change.name !== change.table ? change.name : '',
                      ]
                        .filter(Boolean)
                        .join('.')}
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
