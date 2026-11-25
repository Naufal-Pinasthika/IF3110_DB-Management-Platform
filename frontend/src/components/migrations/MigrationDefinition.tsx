import { useId } from 'react'
import type { MigrationDraft } from './types'

export function MigrationDefinition({ value, onChange, onSave }: {
  value: MigrationDraft
  onChange: (value: MigrationDraft) => void
  onSave: () => void
}) {
  const id = useId()
  const update = <K extends keyof MigrationDraft>(key: K, next: MigrationDraft[K]) => onChange({ ...value, [key]: next })
  return <form className="m2-definition" onSubmit={event => { event.preventDefault(); onSave() }}>
    <section className="panel">
      <div className="panel-toolbar"><div><h2>Migration definition</h2><p>Define the change and how it can be reversed.</p></div><span className="badge">Draft</span></div>
      <label htmlFor={`${id}-name`}>Migration name</label>
      <input id={`${id}-name`} required maxLength={200} value={value.name} onChange={event => update('name', event.target.value)} placeholder="e.g. Add customer email index" />
      <div className="m2-sql-grid">
        <div><label htmlFor={`${id}-up`}>SQL up</label><p className="helper-text">Apply changes to schema or data.</p><textarea id={`${id}-up`} className="sql-editor" required spellCheck={false} value={value.sqlUp} onChange={event => update('sqlUp', event.target.value)} placeholder="-- SQL to apply this migration" /></div>
        <div><label htmlFor={`${id}-down`}>SQL down</label><p className="helper-text">Reverse the committed changes.</p><textarea id={`${id}-down`} className="sql-editor" required={!value.irreversible} disabled={value.irreversible} spellCheck={false} value={value.sqlDown} onChange={event => update('sqlDown', event.target.value)} placeholder={value.irreversible ? 'Not available for an irreversible migration.' : '-- SQL to reverse this migration'} /></div>
      </div>
      <label className="checkbox-label"><input type="checkbox" checked={value.irreversible} onChange={event => update('irreversible', event.target.checked)} />Mark as irreversible</label>
      {value.irreversible && <><label htmlFor={`${id}-reason`}>Reason this migration cannot be reversed</label><textarea id={`${id}-reason`} required rows={3} value={value.irreversibleReason} onChange={event => update('irreversibleReason', event.target.value)} placeholder="e.g. Deleted records cannot be recovered." /></>}
      <p className="helper-text">SQL up and down are fixed after the first execution. Further changes require a new migration.</p>
    </section>
    <section className="panel section-gap">
      <div className="panel-toolbar"><div><h2>Execution mode</h2><p>All migrations run as background jobs after approval.</p></div></div>
      <fieldset className="m2-mode-options"><legend className="sr-only">Execution mode</legend>
        <label className="permission-option"><input type="radio" name={`${id}-mode`} checked={value.mode === 'regular'} onChange={() => update('mode', 'regular')} /><span><strong>Regular migration</strong><span className="cell-subtitle">Run SQL in a single transaction.</span></span></label>
        <label className="permission-option"><input type="radio" name={`${id}-mode`} checked={value.mode === 'batch'} onChange={() => update('mode', 'batch')} /><span><strong>Batch migration</strong><span className="cell-subtitle">Process a large dataset in smaller transactions.</span></span></label>
      </fieldset>
      {value.mode === 'batch' && <div className="m2-batch-fields"><div className="form-columns">
        <div><label htmlFor={`${id}-attribute`}>Cursor attribute</label><input id={`${id}-attribute`} required value={value.batchAttribute} onChange={event => update('batchAttribute', event.target.value)} placeholder="id" /></div>
        <div><label htmlFor={`${id}-size`}>Rows per batch</label><input id={`${id}-size`} type="number" required min={1} step={1} value={value.batchSize || ''} onChange={event => update('batchSize', Number(event.target.value))} /></div>
      </div><p className="helper-text">Use a unique, stable column. SQL must target a bounded batch; completed batches remain committed if a later batch fails.</p></div>}
      <div className="editor-footer"><span>Draft changes stay in this preview until you leave or refresh.</span><div className="toolbar-actions"><button type="button" className="button" disabled title="Validation will be available when migrations are connected.">Validate SQL</button><button type="submit" className="button primary">Save draft locally</button></div></div>
    </section>
  </form>
}
