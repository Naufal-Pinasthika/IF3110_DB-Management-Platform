// Keep preview models separate until the migration API is connected
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

// Status values shown in the queue and execution history
export type ExecutionStatus =
  | 'PENDING'
  | 'RUNNING'
  | 'SUCCEEDED'
  | 'FAILED'
  | 'PARTIAL'
  | 'PARTIALLY_REVERTED'
  | 'UNKNOWN'

// Approval rows displayed in the review preview
export type Approval = {
  id: string
  group: string
  leader: string
  access: 'VIEW_ONLY' | 'VIEW_EDIT'
  status: 'PENDING_REVIEW' | 'APPROVED' | 'REJECTED'
  comment: string
}

// Execution data shown by the queue and history preview
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

// Start new preview drafts with stable defaults for batch mode
export const emptyDraft: MigrationDraft = {
  name: '',
  sqlUp: '',
  sqlDown: '',
  irreversible: false,
  irreversibleReason: '',
  mode: 'regular',
  batchAttribute: 'id',
  batchSize: 1000,
}
