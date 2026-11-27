import { useState } from 'react'
import { formatDate } from '../../api'
import { Badge, EmptyRow, Modal } from '../ui'
import type { Execution } from './types'

// Show one selected job with its last committed checkpoint
export function ExecutionProgress({
  execution,
  canEdit = false,
  busy = false,
  onResume,
  onCancelBatch,
}: {
  execution?: Execution
  canEdit?: boolean
  busy?: boolean
  onResume?: (executionId: string) => unknown
  onCancelBatch?: (executionId: string) => unknown
}) {
  const canResume = Boolean(
    canEdit &&
    onResume &&
    execution &&
    (execution.status === 'PARTIAL' || execution.status === 'PARTIALLY_REVERTED')
  )
  const canCancel = Boolean(
    canEdit &&
    onCancelBatch &&
    execution &&
    execution.status === 'PARTIAL' &&
    (execution.action === 'UP' || execution.action === 'FORWARD' || execution.action === 'RETRY')
  )

  return (
    <section className="panel section-gap">
      <div className="panel-toolbar">
        <div>
          <h2>Execution details</h2>
          <p>
            {execution
              ? execution.migration
              : 'Select a job to inspect its progress and checkpoint.'}
          </p>
        </div>
        {execution && <Badge value={execution.status} />}
      </div>
      <dl className="m2-execution-metadata">
        <div>
          <dt>Job ID</dt>
          <dd className="mono">{execution?.jobId || '—'}</dd>
        </div>
        <div>
          <dt>Execution ID</dt>
          <dd className="mono">{execution?.id || '—'}</dd>
        </div>
        <div>
          <dt>Attempt</dt>
          <dd>{execution?.attempt ?? '—'}</dd>
        </div>
        <div>
          <dt>Committed batches</dt>
          <dd>{execution?.committedBatches ?? '—'}</dd>
        </div>
        <div>
          <dt>Committed rows</dt>
          <dd>{execution?.committedRows.toLocaleString() ?? '—'}</dd>
        </div>
        <div>
          <dt>Last committed cursor</dt>
          <dd className="mono">{execution?.cursor || '—'}</dd>
        </div>
      </dl>
      {execution?.totalBatches ? (
        <>
          <progress
            className="m2-progress"
            aria-label="Committed batches"
            max={execution.totalBatches}
            value={execution.committedBatches}
          />
          <p className="helper-text">
            {execution.committedBatches} of {execution.totalBatches} batches committed.
          </p>
        </>
      ) : (
        <p className="helper-text">Batch progress will appear when an execution is selected.</p>
      )}
      {execution?.status === 'PARTIAL' && (
        <p className="notice">
          Some batches are committed. Resume from the checkpoint or request compensation.
        </p>
      )}
      {execution?.status === 'PARTIALLY_REVERTED' && (
        <p className="notice">Compensation is incomplete. Continue from the last reverted batch.</p>
      )}
      <div className="toolbar-actions">
        <button
          className="button"
          type="button"
          disabled={!canResume || busy}
          onClick={() => execution && onResume?.(execution.id)}
        >
          {busy ? 'Working…' : 'Request resume'}
        </button>
        <button
          className="button danger"
          type="button"
          disabled={!canCancel || busy}
          onClick={() => execution && onCancelBatch?.(execution.id)}
        >
          {busy ? 'Working…' : 'Request batch compensation'}
        </button>
      </div>
      {!onResume && !onCancelBatch && (
        <p className="helper-text">Recovery actions become available for eligible jobs after review.</p>
      )}
    </section>
  )
}

// Filter queued jobs and pass the selected job to the progress panel
export function MigrationQueue({ jobs = [] }: { jobs?: Execution[] }) {
  const [status, setStatus] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const visible = jobs.filter(job => !status || job.status === status)
  const selected = jobs.find(job => job.id === selectedId)

  return (
    <>
      <section className="panel">
        <div className="panel-toolbar">
          <div>
            <h2>Migration queue</h2>
            <p>One migration runs per database. Other databases can run in parallel.</p>
          </div>
          <label className="m2-inline-field">
            Status
            <select value={status} onChange={event => setStatus(event.target.value)}>
              <option value="">All statuses</option>
              {[
                'PENDING',
                'RUNNING',
                'SUCCEEDED',
                'FAILED',
                'PARTIAL',
                'PARTIALLY_REVERTED',
                'UNKNOWN',
              ].map(value => (
                <option key={value} value={value}>
                  {value.toLowerCase().replaceAll('_', ' ')}
                </option>
              ))}
            </select>
          </label>
        </div>
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Job / execution</th>
                <th>Migration</th>
                <th>Operation</th>
                <th>Status</th>
                <th>Committed rows</th>
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              {visible.map(job => (
                <tr key={job.id}>
                  <td className="mono">
                    {job.jobId || '—'}
                    <span className="cell-subtitle">Execution: {job.id}</span>
                  </td>
                  <td>{job.migration}</td>
                  <td>{job.action}</td>
                  <td><Badge value={job.status} /></td>
                  <td>{job.committedRows.toLocaleString()}</td>
                  <td>
                    <button
                      type="button"
                      className="button small"
                      onClick={() => setSelectedId(job.id)}
                    >
                      View progress
                    </button>
                  </td>
                </tr>
              ))}
              {!visible.length && (
                <EmptyRow columns={6}>
                  {jobs.length
                    ? 'No jobs match this status.'
                    : 'No jobs queued. Approved migrations will appear here.'}
                </EmptyRow>
              )}
            </tbody>
          </table>
        </div>
      </section>
      <ExecutionProgress execution={selected} />
    </>
  )
}

// Show only successful work and optionally hide rollback pairs
export function MigrationHistory({ executions = [] }: { executions?: Execution[] }) {
  const [hideRollback, setHideRollback] = useState(false)
  const [selected, setSelected] = useState<Execution | null>(null)
  const committed = executions.filter(item => item.status === 'SUCCEEDED')

  // Hide both the rollback and the exact execution that it cancels
  const hiddenIds = new Set(
    committed
      .filter(item => item.action === 'ROLLBACK' && item.rolledBackExecutionId)
      .flatMap(item => [item.id, item.rolledBackExecutionId!]),
  )
  const visible = committed.filter(item => !hideRollback || !hiddenIds.has(item.id))

  return (
    <section className="panel">
      <div className="panel-toolbar">
        <div>
          <h2>Migration history</h2>
          <p>Committed changes, including successful rollback and forward executions.</p>
        </div>
        <label className="checkbox-label">
          <input
            type="checkbox"
            checked={hideRollback}
            onChange={event => setHideRollback(event.target.checked)}
          />
          Hide rollback
        </label>
      </div>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Execution</th>
              <th>Migration / revision</th>
              <th>Operation</th>
              <th>Executed by</th>
              <th>Time</th>
              <th>SQL snapshot</th>
            </tr>
          </thead>
          <tbody>
            {visible.map(execution => (
              <tr key={execution.id}>
                <td className="mono">{execution.id}</td>
                <td>
                  {execution.migration}
                  <span className="cell-subtitle">Revision {execution.revision}</span>
                </td>
                <td>{execution.action}</td>
                <td>{execution.actor}</td>
                <td>{formatDate(execution.startedAt)}</td>
                <td>
                  <button
                    type="button"
                    className="button small"
                    onClick={() => setSelected(execution)}
                  >
                    View SQL
                  </button>
                </td>
              </tr>
            ))}
            {!visible.length && (
              <EmptyRow columns={6}>
                {committed.length
                  ? 'No committed executions match this filter.'
                  : 'No committed executions yet. History will appear after a migration succeeds.'}
              </EmptyRow>
            )}
          </tbody>
        </table>
      </div>
      <p className="helper-text">
        Hide rollback hides a successful rollback and the exact execution it cancels. History is
        retained. Failed attempts remain available in the queue and application logs.
      </p>
      {selected && (
        <Modal
          title="Executed SQL"
          className="wide-dialog"
          onClose={() => setSelected(null)}
        >
          <p className="helper-text">{selected.id} · Revision {selected.revision}</p>
          <pre className="m2-sql-snapshot"><code>{selected.sql}</code></pre>
          {selected.rolledBackExecutionId && (
            <p className="helper-text">
              Reverses execution {selected.rolledBackExecutionId}.
            </p>
          )}
        </Modal>
      )}
    </section>
  )
}

// Show which operation will wait for group leader approval
export function MigrationActionDialog({
  action,
  onClose,
}: {
  action: 'rollback' | 'forward'
  onClose: () => void
}) {
  const rollback = action === 'rollback'

  return (
    <Modal title={rollback ? 'Request rollback' : 'Request forward migration'} onClose={onClose}>
      <p>
        {rollback
          ? 'Reverse the latest active migration using its original SQL down.'
          : 'Reapply the next rolled-back migration using its original SQL up.'}
      </p>
      <div className="m2-action-target">
        <strong>No eligible migration selected</strong>
        <p className="helper-text">
          {rollback
            ? 'A reversible migration must be applied before it can be rolled back.'
            : 'A migration must be rolled back before it can be forwarded.'}
        </p>
      </div>
      <p className="helper-text">
        All required group leaders must approve this operation before it is queued.
      </p>
      <div className="form-actions">
        <button className="button" type="button" onClick={onClose}>Cancel</button>
        <button className="button primary" type="button" disabled>Submit for review</button>
      </div>
    </Modal>
  )
}
