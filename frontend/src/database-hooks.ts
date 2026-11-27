import { useQuery } from '@tanstack/react-query'
import { useOutletContext } from 'react-router'
import { api, databaseAPI, encode } from './api'
import type { Database, NamedObject } from './types'

// Cache database details separately for every database id
export function useDatabaseQuery(id: string) {
  return useQuery({
    queryKey: ['database', id],
    queryFn: ({ signal }) => api<Database>(databaseAPI(id), { signal }),
    enabled: Boolean(id),
  })
}

// Read the database supplied by its parent route
export function useDatabase() {
  return useOutletContext<Database>()
}

// Load schema names for the selected database
export function useSchemas(id: string) {
  return useQuery({
    queryKey: ['schemas', id],
    queryFn: ({ signal }) => api<NamedObject[]>(`${databaseAPI(id)}/schemas`, { signal }),
  })
}

// Include both ids in the query key so schemas never share table results
export function useTables(id: string, schema: string, enabled = true) {
  return useQuery({
    queryKey: ['tables', id, schema],
    queryFn: ({ signal }) => api<NamedObject[]>(`${databaseAPI(id)}/schemas/${encode(schema)}/tables`, { signal }),
    enabled: Boolean(schema) && enabled,
  })
}

