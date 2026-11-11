import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import { api, APIError } from './api'
import type { User } from './types'

export function useSession() {
  const client = useQueryClient()
  return useQuery({
    queryKey: ['session'],
    queryFn: async ({ signal }): Promise<User | null> => {
      try { return await api<User>('/api/v1/auth/me', { signal }) }
      catch (error) {
        if (error instanceof APIError && error.status === 401) {
          window.dispatchEvent(new Event('discard-changes'))
          client.removeQueries({ predicate: query => query.queryKey[0] !== 'session' })
          return null
        }
        throw error
      }
    },
    staleTime: 30_000,
    refetchOnWindowFocus: true,
  })
}

export async function resetSession(client: QueryClient, user: User | null) {
  await client.cancelQueries()
  // Keep the session Query instance so mounted route guards see the change.
  client.setQueryData<User | null>(['session'], user)
  client.removeQueries({ predicate: query => query.queryKey[0] !== 'session' })
}

