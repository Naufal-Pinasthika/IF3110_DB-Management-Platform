// UI models are intentionally separate from the API types until M2 integration.
export type MigrationDraft = {
  name: string
  sqlUp: string
  sqlDown: string
  irreversible: boolean
  irreversibleReason: string
  mode: 'regular' | 'batch'
  batchAttribute: string
  batchSize: number
}

export type ExecutionStatus = 'PENDING' | 'RUNNING' | 'SUCCEEDED' | 'FAILED' | 'PARTIAL' | 'PARTIALLY_REVERTED' | 'UNKNOWN'

export type Approval = {
  id: string
  group: string
  leader: string
  access: 'VIEW_ONLY' | 'VIEW_EDIT'
  status: 'PENDING_REVIEW' | 'APPROVED' | 'REJECTED'
  comment: string
}

export type Execution = {
  id: string
  jobId?: string
  migration: string
  revision: string
  action: 'UP' | 'ROLLBACK' | 'FORWARD'
  actor: string
  status: ExecutionStatus
  startedAt: string
  sql: string
  rolledBackExecutionId?: string
  committedBatches: number
  committedRows: number
  totalBatches?: number
  cursor?: string
  attempt: number
}

export const emptyDraft: MigrationDraft = {
  name: '', sqlUp: '', sqlDown: '', irreversible: false, irreversibleReason: '',
  mode: 'regular', batchAttribute: 'id', batchSize: 1000,
}
