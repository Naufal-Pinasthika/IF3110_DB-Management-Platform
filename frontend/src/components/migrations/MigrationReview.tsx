import { useState } from 'react'
import { Badge, EmptyRow } from '../ui'
import type { Approval } from './types'

export function MigrationReview({ approvals = [] }: { approvals?: Approval[] }) {
  const [comment, setComment] = useState('')
  return <>
    <section className="panel">
      <div className="panel-toolbar"><div><h2>Validation &amp; review</h2><p>Every group with database access must have its leader approve the change.</p></div><Badge value="DRAFT" /></div>
      <div className="m2-review-summary"><div><strong>SQL validation</strong><p className="helper-text">Not run. Save a migration before requesting validation.</p></div><div><strong>Required approvals</strong><p className="helper-text">Includes leaders of groups with read-only and migration access.</p></div></div>
      <div className="table-scroll"><table><thead><tr><th>Group</th><th>Leader</th><th>Database access</th><th>Decision</th><th>Comment</th></tr></thead><tbody>
        {approvals.map(approval => <tr key={approval.id}><td>{approval.group}</td><td>{approval.leader}</td><td><Badge value={approval.access} /></td><td><Badge value={approval.status} /></td><td>{approval.comment || '—'}</td></tr>)}
        {!approvals.length && <EmptyRow columns={5}>No review requested. Required groups and leaders will appear here.</EmptyRow>}
      </tbody></table></div>
      <p className="helper-text">Rollback, forward, resume, and batch compensation require their own approval before entering the queue.</p>
    </section>
    <section className="panel section-gap">
      <div className="panel-toolbar"><div><h2>Review decision</h2><p>Inspect the migration and leave feedback for its author.</p></div></div>
      <label htmlFor="m2-review-comment">Review comment</label><textarea id="m2-review-comment" rows={4} value={comment} onChange={event => setComment(event.target.value)} placeholder="Explain a concern or record why the change is safe." />
      <div className="editor-footer"><span>No migration is awaiting your decision.</span><div className="toolbar-actions"><button className="button danger" type="button" disabled>Request changes</button><button className="button primary" type="button" disabled>Approve change</button></div></div>
      <div className="m2-review-actions"><button className="button" type="button" disabled>Submit for review</button><button className="button primary" type="button" disabled>Add approved change to queue</button></div>
    </section>
  </>
}
